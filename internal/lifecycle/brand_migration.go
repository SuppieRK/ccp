package lifecycle

import (
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/SuppieRK/cmdshape/internal/filtertrust"
	"github.com/SuppieRK/cmdshape/internal/lifecycle/agents"
	"github.com/SuppieRK/cmdshape/internal/product"
	"github.com/SuppieRK/cmdshape/internal/projectfiles"
	"github.com/SuppieRK/cmdshape/internal/workspaces"
)

const brandMigrationID = "previous-installation-cleanup-v1"

const (
	legacyExecutableName = "ccp"
	legacyProjectDir     = ".ccp"
	legacyConfigDir      = "ccp"
	legacyMainPackage    = "go-command-compression-proxy/cmd/ccp"
	legacyTrustDomain    = "ccp-project-filter-trust-v1"
	legacyReplayPrefix   = "@ccp/base64:"
	currentReplayPrefix  = "@cmdshape/base64:"
	currentSchemaURL     = "https://raw.githubusercontent.com/SuppieRK/cmdshape/refs/heads/main/schemas/cmdshape-filter.schema.json"
	legacyRawSchemaURL   = "https://raw.githubusercontent.com/SuppieRK/ccp/refs/heads/main/schemas/ccp-filter.schema.json"
	legacySchemaURL      = "https://go-command-compression-proxy.dev/schemas/ccp-filter.schema.json"
)

var legacyFilterScaffoldReplacements = []struct {
	legacy  string
	current string
}{
	{
		legacy:  "# canonical id used by .ccp/filters/.mappings.yaml, benchmark fixtures, and current filename.",
		current: "# canonical id used by .cmdshape/filters/.mappings.yaml, benchmark fixtures, and current filename.",
	},
	{
		legacy:  "# flags_consuming_next_arg lists tool flags that consume the next argv token when CCP\n# decides whether a token is a real positional argument.",
		current: "# flags_consuming_next_arg lists tool flags that consume the next argv token when cmdshape\n# decides whether a token is a real positional argument.",
	},
	{
		legacy:  "# 3. compress_output rewrites stdout/stderr/combined output through the fixed CCP DSL",
		current: "# 3. compress_output rewrites stdout/stderr/combined output through the fixed cmdshape DSL",
	},
}

type brandMigrationState string

const (
	brandMigrationCompleted brandMigrationState = "completed"
	brandMigrationSkipped   brandMigrationState = "skipped"
)

type brandMigrationLocation struct {
	Kind   string              `json:"kind"`
	Source string              `json:"source"`
	Target string              `json:"target"`
	State  brandMigrationState `json:"state"`
	Reason string              `json:"reason,omitzero"`
}

type brandMigrationJournal struct {
	ID        string                   `json:"id"`
	UpdatedAt time.Time                `json:"updated_at"`
	Complete  bool                     `json:"complete"`
	Locations []brandMigrationLocation `json:"locations"`
}

var (
	brandMigrationHomeDir                  = os.UserHomeDir
	brandMigrationConfigDir                = os.UserConfigDir
	brandMigrationWorkingDir               = os.Getwd
	brandMigrationExecutable               = os.Executable
	brandMigrationLookPath                 = exec.LookPath
	brandMigrationReadBuildInfo            = buildinfo.ReadFile
	brandMigrationCandidatePaths           = legacyExecutableCandidates
	brandMigrationStdout         io.Writer = os.Stdout
	brandMigrationNow                      = func() time.Time { return time.Now().UTC() }
)

func RunBrandMigrationAuto() error {
	// Startup migration is deliberately best-effort. A blocked home directory,
	// stale registry, or conflicting legacy path must not prevent native command
	// execution; `cmdshape migrate retry` exposes the actionable error instead.
	executable, err := brandMigrationExecutable()
	if err != nil || !isProductExecutable(executable) {
		return nil
	}
	if journal, readErr := readBrandMigrationJournal(); readErr == nil && journal.Complete {
		return nil
	}
	_, _ = executeBrandMigration()
	return nil
}

func isProductExecutable(path string) bool {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return strings.EqualFold(base, product.Name)
}

func RunBrandMigration(args []string) error {
	fs := newLifecycleFlagSet("migrate")
	setLifecycleUsage(
		fs,
		"inspect or retry previous-installation cleanup",
		[]string{
			"cmdshape migrate status",
			"cmdshape migrate retry",
		},
		"Automatic migration is best-effort and never blocks wrapped command execution.",
		"Current cmdshape state wins when both old and new paths exist.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	args = fs.Args()
	if len(args) != 1 {
		return errors.New("usage: cmdshape migrate status|retry")
	}
	switch args[0] {
	case "status":
		journal, err := readBrandMigrationJournal()
		if err != nil {
			return err
		}
		return writeBrandMigrationStatus(journal)
	case "retry":
		journal, err := executeBrandMigration()
		if err != nil {
			return err
		}
		return writeBrandMigrationStatus(journal)
	default:
		return errors.New("usage: cmdshape migrate status|retry")
	}
}

func executeBrandMigration() (brandMigrationJournal, error) {
	home, err := brandMigrationHomeDir()
	if err != nil {
		return brandMigrationJournal{}, err
	}
	currentDir, err := brandMigrationWorkingDir()
	if err != nil {
		return brandMigrationJournal{}, err
	}

	legacyRegistry := filepath.Join(legacyHomeConfigPath(home), "workspaces.db")
	newRegistry := filepath.Join(product.HomeConfigPath(home), "workspaces.db")
	workspaceRoots, registryLocations := registeredWorkspaceRoots(legacyRegistry, newRegistry, currentDir)

	locations := make([]brandMigrationLocation, 0, len(workspaceRoots)+6)
	locations = append(locations, registryLocations...)
	locations = append(locations, migrateLegacyExecutables(home, currentDir)...)
	projectLocations, migratedProjectRoots := migrateRegisteredProjectStates(workspaceRoots)
	locations = append(locations, projectLocations...)
	configLocations, migratedConfigRoots, homeMoved := migrateConfigState(home)
	locations = append(locations, configLocations...)

	if err := workspaces.RewriteProjectStateDir(newRegistry, legacyProjectDir, product.ProjectDir); err != nil {
		locations = append(locations, brandMigrationLocation{
			Kind:   "workspace-registry",
			Source: newRegistry,
			Target: newRegistry,
			State:  brandMigrationSkipped,
			Reason: err.Error(),
		})
	}
	for _, root := range workspaceRoots {
		if location, ok := migrateAgentIntegrations(root, home); ok {
			locations = append(locations, location)
		}
	}
	locations = append(locations, migrateRetiredAgentIntegrations(currentDir, home))
	locations = append(
		locations,
		migrateLegacyFilterHeaders(home, migratedProjectRoots, homeMoved),
		migrateLegacyTrustApprovals(home, homeMoved),
		migrateLegacyRecoveryPayloads(migratedConfigRoots),
	)

	journal := brandMigrationJournal{
		ID:        brandMigrationID,
		UpdatedAt: brandMigrationNow(),
		Complete:  brandMigrationIsComplete(locations),
		Locations: locations,
	}
	if err := writeBrandMigrationJournal(home, journal); err != nil {
		return brandMigrationJournal{}, err
	}
	return journal, nil
}

func migrateRegisteredProjectStates(workspaceRoots []string) ([]brandMigrationLocation, []string) {
	locations := make([]brandMigrationLocation, 0, len(workspaceRoots))
	migratedRoots := make([]string, 0, len(workspaceRoots))
	for _, root := range workspaceRoots {
		location, moved := migrateProjectState(root)
		locations = append(locations, location)
		if moved {
			migratedRoots = append(migratedRoots, root)
		}
	}
	return locations, migratedRoots
}

func migrateConfigState(home string) ([]brandMigrationLocation, []string, bool) {
	currentHome := product.HomeConfigPath(home)
	locations, homeMoved := migrateHomeDirectory("home", legacyHomeConfigPath(home), currentHome)
	locations = append(locations, purgeLegacyLocation("obsolete-home", filepath.Join(home, legacyProjectDir)))

	migratedRoots := make([]string, 0, 2)
	if homeMoved {
		migratedRoots = append(migratedRoots, currentHome)
	}
	platformConfig, err := brandMigrationConfigDir()
	if err != nil {
		return locations, migratedRoots, homeMoved
	}
	legacyPlatformConfig := filepath.Join(platformConfig, legacyConfigDir)
	if sameCleanPath(legacyPlatformConfig, legacyHomeConfigPath(home)) {
		return locations, migratedRoots, homeMoved
	}
	currentPlatformConfig := filepath.Join(platformConfig, product.ConfigDir)
	platformLocations, platformMoved := migrateHomeDirectory(
		"platform-home",
		legacyPlatformConfig,
		currentPlatformConfig,
	)
	locations = append(locations, platformLocations...)
	if platformMoved {
		migratedRoots = append(migratedRoots, currentPlatformConfig)
	}
	return locations, migratedRoots, homeMoved
}

func migrateRetiredAgentIntegrations(root, home string) brandMigrationLocation {
	location := brandMigrationLocation{Kind: "retired-integrations", Source: home, Target: home}
	if err := agents.CleanupRetiredLegacyArtifacts(agents.Context{ScopeRoot: root, HomeDir: home}); err != nil {
		location.State = brandMigrationSkipped
		location.Reason = err.Error()
		return location
	}
	location.State = brandMigrationCompleted
	return location
}

func migrateAgentIntegrations(root, home string) (brandMigrationLocation, bool) {
	adapters, err := agents.NewBuiltInAdapters()
	if err != nil {
		return brandMigrationLocation{
			Kind: "integration", Source: root, Target: root, State: brandMigrationSkipped, Reason: err.Error(),
		}, true
	}
	ctx := agents.Context{ScopeRoot: root, HomeDir: home}
	detected := agents.DetectTools(root, adapters)
	legacyTools := make([]string, 0, len(detected))
	for _, tool := range detected {
		if agents.HasLegacyArtifacts(ctx, tool) {
			legacyTools = append(legacyTools, tool)
		}
	}
	if len(legacyTools) == 0 {
		return brandMigrationLocation{}, false
	}
	for _, tool := range legacyTools {
		adapter := adapters[tool]
		if _, err := adapter.Install(ctx, writeManagedBytes); err != nil {
			return brandMigrationLocation{
				Kind: "integration", Source: root, Target: root, State: brandMigrationSkipped,
				Reason: fmt.Sprintf("%s install: %v", tool, err),
			}, true
		}
		if err := adapter.Verify(ctx); err != nil {
			return brandMigrationLocation{
				Kind: "integration", Source: root, Target: root, State: brandMigrationSkipped,
				Reason: fmt.Sprintf("%s verify: %v", tool, err),
			}, true
		}
	}
	if err := agents.CleanupLegacyArtifacts(ctx, legacyTools); err != nil {
		return brandMigrationLocation{
			Kind: "integration", Source: root, Target: root, State: brandMigrationSkipped, Reason: err.Error(),
		}, true
	}
	return brandMigrationLocation{
		Kind: "integration", Source: root, Target: root, State: brandMigrationCompleted,
	}, true
}

func registeredWorkspaceRoots(legacyRegistry, currentRegistry, currentDir string) ([]string, []brandMigrationLocation) {
	seen := make(map[string]struct{}, 3)
	roots := make([]string, 0, 3)
	locations := make([]brandMigrationLocation, 0, 2)
	for _, registryPath := range []string{currentRegistry, legacyRegistry} {
		entries, err := workspaces.ListPath(registryPath)
		if err != nil {
			locations = append(locations, brandMigrationLocation{
				Kind:   "workspace-registry-scan",
				Source: registryPath,
				Target: currentRegistry,
				State:  brandMigrationSkipped,
				Reason: fmt.Sprintf("read workspace registry: %v", err),
			})
			continue
		}
		for _, entry := range entries {
			if _, ok := seen[entry.CWD]; ok {
				continue
			}
			seen[entry.CWD] = struct{}{}
			roots = append(roots, entry.CWD)
		}
	}
	if currentDir != "" {
		clean := filepath.Clean(currentDir)
		if _, ok := seen[clean]; !ok {
			roots = append(roots, clean)
		}
	}
	slices.Sort(roots)
	return roots, locations
}

func migrateProjectState(root string) (brandMigrationLocation, bool) {
	source := legacyProjectStatePath(root)
	target := product.ProjectStatePath(root)
	sourceInfo, sourceErr := os.Lstat(source)
	_, targetErr := os.Lstat(target)
	canMoveWholeTree := sourceErr == nil &&
		sourceInfo.IsDir() &&
		sourceInfo.Mode()&os.ModeSymlink == 0 &&
		errors.Is(targetErr, os.ErrNotExist)
	location := migratePath("project", source, target)
	if err := projectfiles.RemoveProductRootGitignoreEntries(root); err != nil {
		location.State = brandMigrationSkipped
		if location.Reason == "" {
			location.Reason = err.Error()
		} else {
			location.Reason = errors.Join(errors.New(location.Reason), err).Error()
		}
	}
	_, movedErr := os.Lstat(target)
	moved := canMoveWholeTree && location.State == brandMigrationCompleted && movedErr == nil
	return location, moved
}

func migrateHomeDirectory(kind, source, target string) ([]brandMigrationLocation, bool) {
	sourceInfo, sourceErr := os.Lstat(source)
	_, targetErr := os.Lstat(target)
	canMoveWholeTree := sourceErr == nil &&
		sourceInfo.IsDir() &&
		sourceInfo.Mode()&os.ModeSymlink == 0 &&
		errors.Is(targetErr, os.ErrNotExist)
	location := migratePath(kind, source, target)
	locations := []brandMigrationLocation{location}
	if !canMoveWholeTree || location.State != brandMigrationCompleted {
		return locations, false
	}
	for _, name := range []string{"audit", "repair.lock", "migrations"} {
		locations = append(locations, purgeLegacyLocation(kind+"-"+name, filepath.Join(target, name)))
	}
	_, movedErr := os.Lstat(target)
	return locations, movedErr == nil
}

func purgeLegacyLocation(kind, path string) brandMigrationLocation {
	location := brandMigrationLocation{Kind: kind, Source: path}
	if err := removeLegacyPath(path); err != nil {
		location.State = brandMigrationSkipped
		location.Reason = err.Error()
		return location
	}
	location.State = brandMigrationCompleted
	return location
}

func migratePath(kind, source, target string) brandMigrationLocation {
	location := brandMigrationLocation{Kind: kind, Source: source, Target: target}
	sourceInfo, err := os.Lstat(source)
	if errors.Is(err, os.ErrNotExist) {
		return completedBrandMigration(location)
	}
	if err != nil {
		return skippedBrandMigration(location, err)
	}
	_, targetErr := os.Lstat(target)
	if targetErr == nil {
		return removeSupersededMigrationSource(location, source)
	}
	if !errors.Is(targetErr, os.ErrNotExist) {
		return skippedBrandMigration(location, targetErr)
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 {
		return removeMigrationSymlink(location, source)
	}
	if sourceInfo.IsDir() {
		if err := rejectSymlinksInTree(source); err != nil {
			return discardUnsafeMigrationSource(location, source, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return skippedBrandMigration(location, err)
	}
	if err := os.Rename(source, target); err != nil {
		return discardUnsafeMigrationSource(location, source, err)
	}
	return completedBrandMigration(location)
}

func completedBrandMigration(location brandMigrationLocation) brandMigrationLocation {
	location.State = brandMigrationCompleted
	return location
}

func skippedBrandMigration(location brandMigrationLocation, err error) brandMigrationLocation {
	location.State = brandMigrationSkipped
	location.Reason = err.Error()
	return location
}

func removeSupersededMigrationSource(location brandMigrationLocation, source string) brandMigrationLocation {
	if err := removeLegacyPath(source); err != nil {
		return skippedBrandMigration(location, fmt.Errorf("remove superseded source: %w", err))
	}
	return completedBrandMigration(location)
}

func removeMigrationSymlink(location brandMigrationLocation, source string) brandMigrationLocation {
	if err := os.Remove(source); err != nil {
		return skippedBrandMigration(location, fmt.Errorf("remove symbolic link: %w", err))
	}
	return completedBrandMigration(location)
}

func discardUnsafeMigrationSource(
	location brandMigrationLocation,
	source string,
	cause error,
) brandMigrationLocation {
	if err := removeLegacyPath(source); err != nil {
		return skippedBrandMigration(location, errors.Join(cause, err))
	}
	return completedBrandMigration(location)
}

func removeLegacyPath(path string) error {
	return os.RemoveAll(path)
}

func rejectSymlinksInTree(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refuse to migrate symbolic link %q", path)
		}
		return nil
	})
}

func migrateLegacyExecutables(home, currentDir string) []brandMigrationLocation {
	currentExecutable, err := brandMigrationExecutable()
	if err != nil {
		return []brandMigrationLocation{{
			Kind: "executable", State: brandMigrationSkipped, Reason: err.Error(),
		}}
	}
	candidates := brandMigrationCandidatePaths(home, currentDir, currentExecutable)
	locations := make([]brandMigrationLocation, 0, len(candidates))
	for _, candidate := range candidates {
		locations = append(locations, purgeLegacyExecutable(candidate, currentExecutable))
	}
	return locations
}

func legacyExecutableCandidates(home, currentDir, currentExecutable string) []string {
	name := legacyExecutableName
	if strings.EqualFold(filepath.Ext(currentExecutable), ".exe") {
		name += ".exe"
	}
	candidates := []string{
		filepath.Join(filepath.Dir(currentExecutable), name),
		filepath.Join(home, ".local", "bin", name),
		filepath.Join(currentDir, "bin", name),
	}
	if runtime.GOOS != "windows" {
		candidates = append(candidates, filepath.Join(string(filepath.Separator), "usr", "local", "bin", name))
	}
	if path, err := brandMigrationLookPath(legacyExecutableName); err == nil {
		candidates = append(candidates, path)
	}
	return uniqueCleanPaths(candidates)
}

func uniqueCleanPaths(paths []string) []string {
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" || slices.ContainsFunc(unique, func(existing string) bool {
			return sameCleanPath(existing, path)
		}) {
			continue
		}
		unique = append(unique, filepath.Clean(path))
	}
	slices.Sort(unique)
	return unique
}

func purgeLegacyExecutable(source, currentExecutable string) brandMigrationLocation {
	location := brandMigrationLocation{Kind: "executable", Source: source}
	info, err := os.Lstat(source)
	if errors.Is(err, os.ErrNotExist) {
		location.State = brandMigrationCompleted
		return location
	}
	if err != nil {
		location.State = brandMigrationSkipped
		location.Reason = err.Error()
		return location
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		location.State = brandMigrationSkipped
		location.Reason = "executable ownership is not verifiable"
		return location
	}
	owned, verifyErr := isOwnedLegacyExecutable(source, currentExecutable)
	if verifyErr != nil {
		location.State = brandMigrationSkipped
		location.Reason = verifyErr.Error()
		return location
	}
	if !owned {
		location.State = brandMigrationSkipped
		location.Reason = "executable ownership is not verifiable"
		return location
	}
	if err := os.Remove(source); err != nil {
		location.State = brandMigrationSkipped
		location.Reason = err.Error()
		return location
	}
	location.State = brandMigrationCompleted
	return location
}

func isOwnedLegacyExecutable(path, currentExecutable string) (bool, error) {
	info, err := brandMigrationReadBuildInfo(path)
	if err == nil && (info.Path == legacyMainPackage || info.Path == "github.com/SuppieRK/cmdshape/cmd/cmdshape") {
		return true, nil
	}
	sourceBytes, sourceErr := os.ReadFile(path)
	if sourceErr != nil {
		return false, sourceErr
	}
	currentBytes, currentErr := os.ReadFile(currentExecutable)
	if currentErr != nil {
		return false, currentErr
	}
	return sha256.Sum256(sourceBytes) == sha256.Sum256(currentBytes), nil
}

func legacyHomeConfigPath(home string) string {
	return filepath.Join(home, ".config", legacyConfigDir)
}

func legacyProjectStatePath(root string) string {
	return filepath.Join(root, legacyProjectDir)
}

func migrateLegacyFilterHeaders(home string, workspaceRoots []string, homeMoved bool) brandMigrationLocation {
	roots := legacyFilterRoots(home, workspaceRoots, homeMoved)
	location := brandMigrationLocation{Kind: "filter-schema", Source: strings.Join(roots, ","), Target: currentSchemaURL}
	for _, root := range uniqueCleanPaths(roots) {
		if err := rewriteLegacyFilterRoot(root); err != nil {
			return skippedBrandMigration(location, err)
		}
	}
	return completedBrandMigration(location)
}

func legacyFilterRoots(home string, workspaceRoots []string, homeMoved bool) []string {
	roots := make([]string, 0, len(workspaceRoots)+1)
	if homeMoved {
		roots = append(roots, filepath.Join(product.HomeConfigPath(home), "filters"))
	}
	for _, root := range workspaceRoots {
		roots = append(roots, filepath.Join(product.ProjectStatePath(root), "filters"))
	}
	return roots
}

func rewriteLegacyFilterRoot(root string) error {
	err := filepath.WalkDir(root, rewriteLegacyFilterEntry)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func rewriteLegacyFilterEntry(path string, entry os.DirEntry, walkErr error) error {
	regular, err := regularMigrationFile(entry, walkErr)
	if err != nil {
		return err
	}
	if !regular {
		return nil
	}
	ext := strings.ToLower(filepath.Ext(entry.Name()))
	if ext != ".yaml" && ext != ".yml" {
		return nil
	}
	return rewriteLegacyFilterFile(path, entry)
}

func rewriteLegacyFilterFile(path string, entry os.DirEntry) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated := rewriteLegacyFilterScaffold(raw)
	if bytes.Equal(raw, updated) {
		return nil
	}
	info, err := entry.Info()
	if err != nil {
		return err
	}
	return projectfiles.AtomicWriteFile(path, updated, info.Mode().Perm())
}

func rewriteLegacyFilterScaffold(raw []byte) []byte {
	updated := bytes.ReplaceAll(raw, []byte(legacyRawSchemaURL), []byte(currentSchemaURL))
	updated = bytes.ReplaceAll(updated, []byte(legacySchemaURL), []byte(currentSchemaURL))
	for _, replacement := range legacyFilterScaffoldReplacements {
		updated = bytes.ReplaceAll(updated, []byte(replacement.legacy), []byte(replacement.current))
	}
	return updated
}

type migrationTrustApproval struct {
	Root      string    `json:"root"`
	Digest    string    `json:"digest"`
	TrustedAt time.Time `json:"trusted_at"`
}

type migrationTrustStore struct {
	Version  int                      `json:"version"`
	Projects []migrationTrustApproval `json:"projects"`
}

func migrateLegacyTrustApprovals(home string, homeMoved bool) brandMigrationLocation {
	path := filepath.Join(product.HomeConfigPath(home), "filter-trust.json")
	location := brandMigrationLocation{Kind: "filter-trust", Source: path, Target: path}
	if !homeMoved {
		return completedBrandMigration(location)
	}
	store, present, err := readMigrationTrustStore(path)
	if err != nil {
		return skippedBrandMigration(location, err)
	}
	if !present {
		return completedBrandMigration(location)
	}

	projects, changed := migrateTrustApprovals(store.Projects)
	if !changed {
		return completedBrandMigration(location)
	}
	store.Projects = projects
	if err := writeMigrationTrustStore(path, store); err != nil {
		return skippedBrandMigration(location, err)
	}
	return completedBrandMigration(location)
}

func readMigrationTrustStore(path string) (migrationTrustStore, bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return migrationTrustStore{}, false, nil
	}
	if err != nil {
		return migrationTrustStore{}, false, err
	}
	var store migrationTrustStore
	if json.Unmarshal(raw, &store) != nil {
		// An invalid cmdshape-side store contains no actionable legacy path or
		// identifier. Leave it for the trust command to report rather than
		// guessing whether it predated the rename.
		return migrationTrustStore{}, false, nil
	}
	return store, true, nil
}

func migrateTrustApprovals(approvals []migrationTrustApproval) ([]migrationTrustApproval, bool) {
	projects := make([]migrationTrustApproval, 0, len(approvals))
	changed := false
	for _, approval := range approvals {
		migrated, keep, approvalChanged := migrateTrustApproval(approval)
		changed = changed || approvalChanged
		if !keep {
			continue
		}
		projects = append(projects, migrated)
	}
	return projects, changed
}

func migrateTrustApproval(approval migrationTrustApproval) (migrationTrustApproval, bool, bool) {
	currentRoot, currentDigest, present, err := filtertrust.ProjectDigest(approval.Root)
	if err != nil || !present {
		return migrationTrustApproval{}, false, true
	}
	if approval.Digest == currentDigest {
		approval.Root = currentRoot
		return approval, true, false
	}
	_, legacyDigest, legacyPresent, err := filtertrust.ProjectDigestForDomain(
		approval.Root,
		legacyTrustDomain,
	)
	if err != nil || !legacyPresent || approval.Digest != legacyDigest {
		return migrationTrustApproval{}, false, true
	}
	approval.Root = currentRoot
	approval.Digest = currentDigest
	return approval, true, true
}

func writeMigrationTrustStore(path string, store migrationTrustStore) error {
	updated, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	updated = append(updated, '\n')
	return projectfiles.AtomicWriteFile(path, updated, 0o600)
}

func migrateLegacyRecoveryPayloads(configRoots []string) brandMigrationLocation {
	roots := make([]string, 0, len(configRoots))
	for _, configRoot := range uniqueCleanPaths(configRoots) {
		roots = append(roots, filepath.Join(configRoot, "recovery"))
	}
	location := brandMigrationLocation{
		Kind:   "recovery-payloads",
		Source: strings.Join(roots, ","),
		Target: strings.Join(roots, ","),
	}
	for _, root := range roots {
		err := rewriteLegacyRecoveryPayloads(root)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			location.State = brandMigrationSkipped
			location.Reason = err.Error()
			return location
		}
	}
	location.State = brandMigrationCompleted
	return location
}

func rewriteLegacyRecoveryPayloads(root string) error {
	return filepath.WalkDir(root, rewriteLegacyRecoveryEntry)
}

func rewriteLegacyRecoveryEntry(path string, entry os.DirEntry, walkErr error) error {
	regular, err := regularMigrationFile(entry, walkErr)
	if err != nil {
		return err
	}
	if !regular {
		return nil
	}
	if entry.Name() != "stdout.txt" && entry.Name() != "stderr.txt" {
		return nil
	}
	return rewriteLegacyRecoveryFile(path, entry)
}

func rewriteLegacyRecoveryFile(path string, entry os.DirEntry) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated := bytes.ReplaceAll(raw, []byte(legacyReplayPrefix), []byte(currentReplayPrefix))
	if bytes.Equal(raw, updated) {
		return nil
	}
	info, err := entry.Info()
	if err != nil {
		return err
	}
	return projectfiles.AtomicWriteFile(path, updated, info.Mode().Perm())
}

func regularMigrationFile(entry os.DirEntry, walkErr error) (bool, error) {
	if errors.Is(walkErr, os.ErrNotExist) {
		return false, nil
	}
	if walkErr != nil {
		return false, walkErr
	}
	return !entry.IsDir() && entry.Type()&os.ModeSymlink == 0, nil
}

func brandMigrationIsComplete(locations []brandMigrationLocation) bool {
	return !slices.ContainsFunc(locations, func(location brandMigrationLocation) bool {
		return location.State != brandMigrationCompleted
	})
}

func brandMigrationJournalPath(home string) string {
	return filepath.Join(product.HomeConfigPath(home), "migrations", brandMigrationID+".json")
}

func writeBrandMigrationJournal(home string, journal brandMigrationJournal) error {
	raw, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	path := brandMigrationJournalPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create migration journal directory: %w", err)
	}
	return projectfiles.AtomicWriteFile(path, raw, 0o600)
}

func readBrandMigrationJournal() (brandMigrationJournal, error) {
	home, err := brandMigrationHomeDir()
	if err != nil {
		return brandMigrationJournal{}, err
	}
	raw, err := os.ReadFile(brandMigrationJournalPath(home))
	if errors.Is(err, os.ErrNotExist) {
		return brandMigrationJournal{ID: brandMigrationID}, nil
	}
	if err != nil {
		return brandMigrationJournal{}, err
	}
	var journal brandMigrationJournal
	if err := json.Unmarshal(raw, &journal); err != nil {
		return brandMigrationJournal{}, err
	}
	return journal, nil
}

func writeBrandMigrationStatus(journal brandMigrationJournal) error {
	status := "pending"
	if journal.Complete {
		status = "complete"
	}
	if _, err := fmt.Fprintf(brandMigrationStdout, "cmdshape migration: %s\n", status); err != nil {
		return err
	}
	for _, location := range journal.Locations {
		if _, err := fmt.Fprintf(
			brandMigrationStdout,
			"- %s: %s (%s -> %s)%s\n",
			location.Kind,
			location.State,
			location.Source,
			location.Target,
			migrationReasonSuffix(location.Reason),
		); err != nil {
			return err
		}
	}
	return nil
}

func migrationReasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return ": " + reason
}

func sameCleanPath(a, b string) bool {
	left := filepath.Clean(a)
	right := filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
