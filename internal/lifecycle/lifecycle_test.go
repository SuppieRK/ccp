package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/lifecycle/agents"
)

type testAdapter struct {
	id   string
	plan []agents.PlannedArtifact
}

type failingInstallAdapter struct{}

type failingVerifyAdapter struct{}
type countingAdapter struct{ installed int }

func (t testAdapter) ID() string {
	return t.id
}

func (t testAdapter) DetectRoot(scopeRoot string) string {
	return scopeRoot
}

func (t testAdapter) Install(_ agents.Context, write agents.WriterFunc) (agents.InstallResult, error) {
	if len(t.plan) == 0 {
		return agents.InstallResult{}, nil
	}
	_, err := write(t.plan[0].Path, []byte(t.plan[0].Content), t.plan[0].Perm)
	if err != nil {
		return agents.InstallResult{}, err
	}
	return agents.InstallResult{Applied: 1}, nil
}

func (t testAdapter) Plan(_ agents.Context) []agents.PlannedArtifact {
	return t.plan
}

func (t testAdapter) Verify(_ agents.Context) error {
	return nil
}

func (failingInstallAdapter) ID() string { return "broken-install" }

func (failingInstallAdapter) DetectRoot(scopeRoot string) string { return scopeRoot }

func (failingInstallAdapter) Install(_ agents.Context, _ agents.WriterFunc) (agents.InstallResult, error) {
	return agents.InstallResult{}, errors.New("install failed")
}

func (failingInstallAdapter) Plan(_ agents.Context) []agents.PlannedArtifact { return nil }

func (failingInstallAdapter) Verify(_ agents.Context) error { return nil }

func (failingVerifyAdapter) ID() string { return "broken-verify" }

func (failingVerifyAdapter) DetectRoot(scopeRoot string) string { return scopeRoot }

func (failingVerifyAdapter) Install(_ agents.Context, _ agents.WriterFunc) (agents.InstallResult, error) {
	return agents.InstallResult{Applied: 1}, nil
}

func (failingVerifyAdapter) Plan(_ agents.Context) []agents.PlannedArtifact { return nil }

func (failingVerifyAdapter) Verify(_ agents.Context) error { return errors.New("verify failed") }

func (c *countingAdapter) ID() string { return "fake" }

func (c *countingAdapter) DetectRoot(scopeRoot string) string { return scopeRoot }

func (c *countingAdapter) Install(_ agents.Context, _ agents.WriterFunc) (agents.InstallResult, error) {
	c.installed++
	return agents.InstallResult{Applied: 1}, nil
}

func (c *countingAdapter) Plan(_ agents.Context) []agents.PlannedArtifact { return nil }

func (c *countingAdapter) Verify(_ agents.Context) error { return nil }

func assertSingleFailedStateWithReason(t *testing.T, states []toolState, reason string) {
	t.Helper()
	if len(states) != 1 || states[0].Status != "failed" {
		t.Fatalf("unexpected states %+v", states)
	}
	if !strings.Contains(states[0].Reason, reason) {
		t.Fatalf("expected failure reason in state, got %+v", states[0])
	}
}

func TestParseToolsSortsAndDedups(t *testing.T) {
	got := parseTools("Zeta,alpha, Alpha , zeta,,")
	if !slices.Equal(got, []string{"alpha", "zeta"}) {
		t.Fatalf("unexpected order %v", got)
	}
}

func TestInitDetectRootUsesWorkingDirectory(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)
	root, err := initDetectRoot()
	if err != nil {
		t.Fatalf("initDetectRoot: %v", err)
	}
	rootResolved, rootErr := filepath.EvalSymlinks(root)
	if rootErr != nil {
		rootResolved = root
	}
	tmpResolved, tmpErr := filepath.EvalSymlinks(tmp)
	if tmpErr != nil {
		tmpResolved = tmp
	}
	if rootResolved != tmpResolved {
		t.Fatalf("unexpected local root %s (resolved %s), want %s (resolved %s)", root, rootResolved, tmp, tmpResolved)
	}
}

func TestInitPathUsesHomeConfig(t *testing.T) {
	home := t.TempDir()
	setHomeDirForTest(t, home)
	path, err := initPath()
	if err != nil {
		t.Fatalf("initPath: %v", err)
	}
	if path == "" {
		t.Fatalf("unexpected empty path")
	}
	assertPathUnderBase(t, home, path)
}

func TestApplyAdapters(t *testing.T) {
	tmp := t.TempDir()
	adapter := testAdapter{
		id: "alpha",
		plan: []agents.PlannedArtifact{
			{Kind: agents.ArtifactHook, Path: filepath.Join(tmp, "hook.sh"), Content: "echo", Perm: 0o644},
		},
	}
	adapters := map[string]agents.Adapter{
		"alpha": adapter,
	}
	ctx := agents.Context{ScopeRoot: tmp, HomeDir: tmp}
	states, err := applyAdapters(ctx, []string{"alpha"}, adapters)
	if err != nil {
		t.Fatalf("applyAdapters: %v", err)
	}
	if len(states) != 1 || states[0].Tool != "alpha" {
		t.Fatalf("unexpected states %+v", states)
	}
}

func TestApplyAdaptersInstallFailureReturnsFailedState(t *testing.T) {
	ctx := agents.Context{ScopeRoot: t.TempDir(), HomeDir: t.TempDir()}
	adapters := map[string]agents.Adapter{
		"broken-install": failingInstallAdapter{},
	}
	states, err := applyAdapters(ctx, []string{"broken-install"}, adapters)
	if err == nil || !strings.Contains(err.Error(), "install failed") {
		t.Fatalf("expected install failure, got err=%v", err)
	}
	assertSingleFailedStateWithReason(t, states, "install failed")
}

func TestApplyAdaptersVerifyFailureReturnsFailedState(t *testing.T) {
	ctx := agents.Context{ScopeRoot: t.TempDir(), HomeDir: t.TempDir()}
	adapters := map[string]agents.Adapter{
		"broken-verify": failingVerifyAdapter{},
	}
	states, err := applyAdapters(ctx, []string{"broken-verify"}, adapters)
	if err == nil || !strings.Contains(err.Error(), "verify failed") {
		t.Fatalf("expected verify failure, got err=%v", err)
	}
	assertSingleFailedStateWithReason(t, states, "verify failed")
}

func TestInitPathWithoutHomeFails(t *testing.T) {
	for _, key := range []string{"HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH"} {
		_ = os.Unsetenv(key)
	}
	if _, err := initPath(); err == nil {
		t.Fatal("expected initPath to fail without a home directory")
	}
}

func TestWriteManagedFileCreateNoopAndBackup(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg", "managed.txt")

	changed, err := writeManagedFile(path, []byte("v1\n"), 0o644)
	if err != nil || !changed {
		t.Fatalf("first write changed=%v err=%v", changed, err)
	}
	changed, err = writeManagedFile(path, []byte("v1\n"), 0o644)
	if err != nil || changed {
		t.Fatalf("second write changed=%v err=%v", changed, err)
	}
	changed, err = writeManagedFile(path, []byte("v2\n"), 0o644)
	if err != nil || !changed {
		t.Fatalf("third write changed=%v err=%v", changed, err)
	}
	matches, err := filepath.Glob(path + ".bak.*")
	if err != nil || len(matches) == 0 {
		t.Fatalf("expected backup file, matches=%v err=%v", matches, err)
	}
}

func TestRunStartupMaintenanceReconcilesStaleConfiguredTools(t *testing.T) {
	ws := newLifecycleWorkspace(t)
	adapter := &countingAdapter{}
	restore := stubStartupMaintenanceDeps(
		func() string { return "new-version" },
		func() map[string]agents.Adapter { return map[string]agents.Adapter{"fake": adapter} },
	)
	defer restore()

	cfgPath, err := initPath()
	if err != nil {
		t.Fatalf("initPath: %v", err)
	}
	if _, err := persistInitConfig(cfgPath, initConfig{
		Tools:              []string{"fake"},
		State:              []toolState{{Tool: "fake", Status: "applied", Reason: "old"}},
		CCPVersion:         "old-version",
		IntegrationVersion: 0,
	}); err != nil {
		t.Fatalf("persist stale config: %v", err)
	}

	chdirForTest(t, ws.work)
	if err := RunStartupMaintenance(); err != nil {
		t.Fatalf("RunStartupMaintenance: %v", err)
	}
	if adapter.installed != 1 {
		t.Fatalf("expected one reconcile install, got %d", adapter.installed)
	}

	cfg, exists, err := readInitConfig(cfgPath)
	if err != nil || !exists {
		t.Fatalf("readInitConfig err=%v exists=%v", err, exists)
	}
	if cfg.CCPVersion != "new-version" || cfg.IntegrationVersion != integrationStateVersion {
		t.Fatalf("unexpected reconcile metadata: %+v", cfg)
	}
}

func TestRunStartupMaintenanceSkipsUpToDateConfig(t *testing.T) {
	ws := newLifecycleWorkspace(t)
	adapter := &countingAdapter{}
	restore := stubStartupMaintenanceDeps(
		func() string { return "current-version" },
		func() map[string]agents.Adapter { return map[string]agents.Adapter{"fake": adapter} },
	)
	defer restore()

	cfgPath, err := initPath()
	if err != nil {
		t.Fatalf("initPath: %v", err)
	}
	if _, err := persistInitConfig(cfgPath, initConfig{
		Tools:              []string{"fake"},
		State:              []toolState{{Tool: "fake", Status: "applied", Reason: "current"}},
		CCPVersion:         "current-version",
		IntegrationVersion: integrationStateVersion,
	}); err != nil {
		t.Fatalf("persist current config: %v", err)
	}

	chdirForTest(t, ws.work)
	if err := RunStartupMaintenance(); err != nil {
		t.Fatalf("RunStartupMaintenance: %v", err)
	}
	if adapter.installed != 0 {
		t.Fatalf("expected no reconcile install, got %d", adapter.installed)
	}
}

func TestRunStartupMaintenanceCleansLegacyProjectInitStateButPreservesGainDB(t *testing.T) {
	ws := newLifecycleWorkspace(t)
	restore := stubStartupMaintenanceDeps(
		func() string { return "current-version" },
		func() map[string]agents.Adapter { return map[string]agents.Adapter{} },
	)
	defer restore()

	ccpDir := filepath.Join(ws.work, ".ccp")
	if err := os.MkdirAll(ccpDir, 0o755); err != nil {
		t.Fatalf("mkdir .ccp: %v", err)
	}
	stale := filepath.Join(ccpDir, initConfigFileName)
	backup := stale + ".bak.123"
	gainDB := filepath.Join(ccpDir, "gain.db")
	for _, file := range []string{stale, backup, gainDB} {
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}

	chdirForTest(t, ws.work)
	if err := RunStartupMaintenance(); err != nil {
		t.Fatalf("RunStartupMaintenance: %v", err)
	}
	for _, removed := range []string{stale, backup} {
		if _, err := os.Stat(removed); !os.IsNotExist(err) {
			t.Fatalf("expected removed legacy file %s, err=%v", removed, err)
		}
	}
	if _, err := os.Stat(gainDB); err != nil {
		t.Fatalf("expected gain.db preserved, err=%v", err)
	}
}

func TestRunStartupMaintenanceSkipsWhenLockAlreadyHeld(t *testing.T) {
	ws := newLifecycleWorkspace(t)
	adapter := &countingAdapter{}
	restore := stubStartupMaintenanceDeps(
		func() string { return "new-version" },
		func() map[string]agents.Adapter { return map[string]agents.Adapter{"fake": adapter} },
	)
	defer restore()

	cfgPath, err := initPath()
	if err != nil {
		t.Fatalf("initPath: %v", err)
	}
	if _, err := persistInitConfig(cfgPath, initConfig{
		Tools:              []string{"fake"},
		State:              []toolState{{Tool: "fake", Status: "applied", Reason: "old"}},
		CCPVersion:         "old-version",
		IntegrationVersion: 0,
	}); err != nil {
		t.Fatalf("persist stale config: %v", err)
	}
	if err := os.WriteFile(cfgPath+".lock", []byte("held"), 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	chdirForTest(t, ws.work)
	if err := RunStartupMaintenance(); err != nil {
		t.Fatalf("RunStartupMaintenance: %v", err)
	}
	if adapter.installed != 0 {
		t.Fatalf("expected reconcile to skip while lock held, got %d", adapter.installed)
	}
}

func stubStartupMaintenanceDeps(versionFn func() string, adaptersFn func() map[string]agents.Adapter) func() {
	prevVersion := startupMaintenanceVersion
	prevAdapters := startupMaintenanceAdapters
	prevPrintf := startupMaintenancePrintf
	startupMaintenanceVersion = versionFn
	startupMaintenanceAdapters = adaptersFn
	startupMaintenancePrintf = func(string, ...any) (int, error) { return 0, nil }
	return func() {
		startupMaintenanceVersion = prevVersion
		startupMaintenanceAdapters = prevAdapters
		startupMaintenancePrintf = prevPrintf
	}
}
