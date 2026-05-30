package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"

	"go-command-compression-proxy/internal/projectfiles"
	"go-command-compression-proxy/internal/version"
)

type migrationSurface string

const (
	migrationSurfaceRepo migrationSurface = "repo"
	migrationSurfaceHome migrationSurface = "home"
)

type migrationContext struct {
	homeDir string
	cwd     string
	mode    repairMode
}

type migration struct {
	id      string
	surface migrationSurface
	version string
	run     func(migrationContext) error
}

var builtInMigrations = []migration{
	{
		id:      "cleanup-legacy-project-init-state",
		surface: migrationSurfaceRepo,
		version: "0.7.0",
		run:     cleanupLegacyProjectInitStateMigration,
	},
	{
		id:      "migrate-repo-local-ccp-gitignore",
		surface: migrationSurfaceRepo,
		version: "0.7.0",
		run:     migrateRepoLocalCCPGitignore,
	},
}

func runMigrations(surface migrationSurface, sinceVersion string, ctx migrationContext) error {
	var since version.Semantic
	if sinceVersion != "" {
		parsed, ok := version.Parse(sinceVersion)
		if !ok {
			return fmt.Errorf("invalid migration version %q: must be X.Y.Z", sinceVersion)
		}
		since = parsed
	}

	for _, current := range builtInMigrations {
		if current.surface != surface {
			continue
		}
		migrationVersion, ok := version.Parse(current.version)
		if !ok {
			return fmt.Errorf("migration %s has invalid version %q", current.id, current.version)
		}
		if sinceVersion != "" {
			if !since.Less(migrationVersion) {
				continue
			}
		}
		if err := current.run(ctx); err != nil {
			return fmt.Errorf("migration %s: %w", current.id, err)
		}
	}
	return nil
}

func cleanupLegacyProjectInitStateMigration(ctx migrationContext) error {
	ccpDir := filepath.Join(ctx.cwd, ".ccp")
	targets := []string{filepath.Join(ccpDir, "init.json")}
	matches, err := filepath.Glob(filepath.Join(ccpDir, "init.json.bak.*"))
	if err != nil {
		return err
	}
	targets = append(targets, matches...)
	for _, path := range targets {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func migrateRepoLocalCCPGitignore(ctx migrationContext) error {
	if !isGitRoot(ctx.cwd) {
		return nil
	}
	ccpDir := filepath.Join(ctx.cwd, ".ccp")
	info, err := os.Stat(ccpDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	if err := projectfiles.EnsureNestedCCPGitignore(ctx.cwd); err != nil {
		return err
	}
	return projectfiles.RemoveLegacyRootCCPGitignoreEntries(ctx.cwd)
}

func isGitRoot(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

func validateBuiltInMigrationVersions() error {
	for _, current := range builtInMigrations {
		if _, ok := version.Parse(current.version); !ok {
			return fmt.Errorf("migration %s has invalid version %q", current.id, current.version)
		}
	}
	return nil
}
