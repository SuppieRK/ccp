package lifecycle

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("lifecycle migrations", func() {
	var ctx migrationContext

	BeforeEach(func() {
		ws := newLifecycleWorkspaceSpec()
		ctx = migrationContext{homeDir: ws.home, cwd: ws.work, mode: repairModePreserve}
	})

	It("runs only migrations matching the requested surface in declaration order", func() {
		var seen []string
		stubBuiltInMigrationsForSpec([]migration{
			{id: "repo-a", surface: migrationSurfaceRepo, version: "0.1.0", run: recordMigration(&seen, "repo-a")},
			{id: "home-a", surface: migrationSurfaceHome, version: "0.1.0", run: recordMigration(&seen, "home-a")},
			{id: "repo-b", surface: migrationSurfaceRepo, version: "0.2.0", run: recordMigration(&seen, "repo-b")},
		})

		Expect(runMigrations(migrationSurfaceRepo, "", ctx)).To(Succeed())

		Expect(seen).To(Equal([]string{"repo-a", "repo-b"}))
	})

	It("runs all matching migrations when no version is provided", func() {
		var seen []string
		stubBuiltInMigrationsForSpec([]migration{
			{id: "old", surface: migrationSurfaceRepo, version: "0.1.0", run: recordMigration(&seen, "old")},
			{id: "new", surface: migrationSurfaceRepo, version: "0.2.0", run: recordMigration(&seen, "new")},
		})

		Expect(runMigrations(migrationSurfaceRepo, "", ctx)).To(Succeed())

		Expect(seen).To(Equal([]string{"old", "new"}))
	})

	It("skips migrations at or below the provided version", func() {
		var seen []string
		stubBuiltInMigrationsForSpec([]migration{
			{id: "old", surface: migrationSurfaceRepo, version: "0.1.0", run: recordMigration(&seen, "old")},
			{id: "equal", surface: migrationSurfaceRepo, version: "0.2.0", run: recordMigration(&seen, "equal")},
			{id: "new", surface: migrationSurfaceRepo, version: "0.3.0", run: recordMigration(&seen, "new")},
		})

		Expect(runMigrations(migrationSurfaceRepo, "0.2.0", ctx)).To(Succeed())

		Expect(seen).To(Equal([]string{"new"}))
	})

	It("rejects an invalid caller version", func() {
		stubBuiltInMigrationsForSpec([]migration{
			{id: "repo", surface: migrationSurfaceRepo, version: "0.1.0", run: func(migrationContext) error { return nil }},
		})

		err := runMigrations(migrationSurfaceRepo, "v0.1.0", ctx)

		Expect(err).To(MatchError(ContainSubstring("invalid migration version")))
	})

	It("rejects an invalid matching migration version", func() {
		stubBuiltInMigrationsForSpec([]migration{
			{id: "broken", surface: migrationSurfaceRepo, version: "dev", run: func(migrationContext) error { return nil }},
		})

		err := runMigrations(migrationSurfaceRepo, "", ctx)

		Expect(err).To(MatchError(ContainSubstring("migration broken has invalid version")))
	})

	It("wraps migration failures with the migration id", func() {
		stubBuiltInMigrationsForSpec([]migration{
			{id: "broken", surface: migrationSurfaceRepo, version: "0.1.0", run: func(migrationContext) error { return errors.New("boom") }},
		})

		err := runMigrations(migrationSurfaceRepo, "", ctx)

		Expect(err).To(MatchError(ContainSubstring("migration broken: boom")))
	})

	It("keeps built-in migration versions valid", func() {
		Expect(validateBuiltInMigrationVersions()).To(Succeed())
	})

	Describe("migrate-repo-local-ccp-gitignore", func() {
		It("migrates exact root gitignore entries after creating nested ignore state", func() {
			Expect(os.Mkdir(filepath.Join(ctx.cwd, ".git"), 0o755)).To(Succeed())
			Expect(os.Mkdir(filepath.Join(ctx.cwd, ".ccp"), 0o755)).To(Succeed())
			rootGitignore := filepath.Join(ctx.cwd, ".gitignore")
			Expect(os.WriteFile(rootGitignore, []byte("node_modules/\n.ccp\n.ccp/\n.ccp/**\n!.ccp/filters/\n"), 0o644)).To(Succeed())

			Expect(migrateRepoLocalCCPGitignore(ctx)).To(Succeed())
			Expect(migrateRepoLocalCCPGitignore(ctx)).To(Succeed())

			nested, err := os.ReadFile(filepath.Join(ctx.cwd, ".ccp", ".gitignore"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(nested)).To(Equal("gain.db\n.gitignore\n"))
			root, err := os.ReadFile(rootGitignore)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(root)).To(Equal("node_modules/\n.ccp/**\n!.ccp/filters/\n"))
		})

		It("noops outside git roots", func() {
			Expect(os.Mkdir(filepath.Join(ctx.cwd, ".ccp"), 0o755)).To(Succeed())
			rootGitignore := filepath.Join(ctx.cwd, ".gitignore")
			Expect(os.WriteFile(rootGitignore, []byte(".ccp\n"), 0o644)).To(Succeed())

			Expect(migrateRepoLocalCCPGitignore(ctx)).To(Succeed())

			_, err := os.Stat(filepath.Join(ctx.cwd, ".ccp", ".gitignore"))
			Expect(err).To(MatchError(os.ErrNotExist))
			root, err := os.ReadFile(rootGitignore)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(root)).To(Equal(".ccp\n"))
		})

		It("noops in git roots without CCP local state", func() {
			Expect(os.Mkdir(filepath.Join(ctx.cwd, ".git"), 0o755)).To(Succeed())
			rootGitignore := filepath.Join(ctx.cwd, ".gitignore")
			Expect(os.WriteFile(rootGitignore, []byte(".ccp\n"), 0o644)).To(Succeed())

			Expect(migrateRepoLocalCCPGitignore(ctx)).To(Succeed())

			_, err := os.Stat(filepath.Join(ctx.cwd, ".ccp", ".gitignore"))
			Expect(err).To(MatchError(os.ErrNotExist))
			root, err := os.ReadFile(rootGitignore)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(root)).To(Equal(".ccp\n"))
		})

		It("does not edit the root gitignore when nested ignore ensure fails", func() {
			Expect(os.Mkdir(filepath.Join(ctx.cwd, ".git"), 0o755)).To(Succeed())
			ccpDir := filepath.Join(ctx.cwd, ".ccp")
			Expect(os.Mkdir(ccpDir, 0o755)).To(Succeed())
			rootGitignore := filepath.Join(ctx.cwd, ".gitignore")
			Expect(os.WriteFile(rootGitignore, []byte(".ccp\n"), 0o644)).To(Succeed())
			outside := filepath.Join(GinkgoT().TempDir(), "outside-gitignore")
			Expect(os.WriteFile(outside, []byte("keep\n"), 0o644)).To(Succeed())
			if err := os.Symlink(outside, filepath.Join(ccpDir, ".gitignore")); err != nil {
				Skip("symlink creation unavailable: " + err.Error())
			}

			err := migrateRepoLocalCCPGitignore(ctx)

			Expect(err).To(HaveOccurred())
			root, readErr := os.ReadFile(rootGitignore)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(root)).To(Equal(".ccp\n"))
		})

		It("produces git-visible filters while ignoring generated state", func() {
			if _, err := exec.LookPath("git"); err != nil {
				Skip("git unavailable: " + err.Error())
			}
			cmd := exec.Command("git", "init")
			cmd.Dir = ctx.cwd
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), string(out))
			ccpDir := filepath.Join(ctx.cwd, ".ccp")
			Expect(os.MkdirAll(filepath.Join(ccpDir, "filters"), 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(ctx.cwd, ".gitignore"), []byte(".ccp\n"), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(ccpDir, "gain.db"), []byte("metrics"), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(ccpDir, "filters", "local.yaml"), []byte("version: 1\n"), 0o644)).To(Succeed())

			Expect(migrateRepoLocalCCPGitignore(ctx)).To(Succeed())

			ignored := gitCheckIgnore(ctx.cwd, ".ccp/gain.db", ".ccp/.gitignore")
			Expect(ignored).To(ContainElement(".ccp/gain.db"))
			Expect(ignored).To(ContainElement(".ccp/.gitignore"))
			Expect(gitCheckIgnore(ctx.cwd, ".ccp/filters/local.yaml")).To(BeEmpty())
		})
	})

	Describe("cleanup-legacy-project-init-state", func() {
		It("removes legacy init files while preserving gain db", func() {
			ccpDir := filepath.Join(ctx.cwd, ".ccp")
			Expect(os.MkdirAll(ccpDir, 0o755)).To(Succeed())
			initPath := filepath.Join(ccpDir, "init.json")
			backupPath := filepath.Join(ccpDir, "init.json.bak.123")
			gainPath := filepath.Join(ccpDir, "gain.db")
			for _, path := range []string{initPath, backupPath, gainPath} {
				Expect(os.WriteFile(path, []byte("x"), 0o644)).To(Succeed())
			}

			Expect(cleanupLegacyProjectInitStateMigration(ctx)).To(Succeed())

			Expect(initPath).NotTo(BeAnExistingFile())
			Expect(backupPath).NotTo(BeAnExistingFile())
			Expect(gainPath).To(BeAnExistingFile())
		})

		It("is idempotent when no legacy files exist", func() {
			Expect(cleanupLegacyProjectInitStateMigration(ctx)).To(Succeed())
			Expect(cleanupLegacyProjectInitStateMigration(ctx)).To(Succeed())
		})
	})
})

func stubBuiltInMigrationsForSpec(migrations []migration) {
	previous := builtInMigrations
	builtInMigrations = migrations
	DeferCleanup(func() { builtInMigrations = previous })
}

func recordMigration(seen *[]string, id string) func(migrationContext) error {
	return func(migrationContext) error {
		*seen = append(*seen, id)
		return nil
	}
}

func gitCheckIgnore(root string, paths ...string) []string {
	args := append([]string{"-C", root, "check-ignore"}, paths...)
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil
		}
		Expect(err).NotTo(HaveOccurred())
	}
	var ignored []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			ignored = append(ignored, line)
		}
	}
	return ignored
}
