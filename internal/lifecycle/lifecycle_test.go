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

func TestInitScopeRootLocal(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)
	root, scope, err := initScopeRoot(false)
	if err != nil {
		t.Fatalf("initScopeRoot: %v", err)
	}
	if scope != "local" {
		t.Fatalf("unexpected scope %s", scope)
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

func TestInitPathGlobal(t *testing.T) {
	home := t.TempDir()
	setHomeDirForTest(t, home)
	path, scope, err := initPath(true)
	if err != nil {
		t.Fatalf("initPath: %v", err)
	}
	if scope != "global" || path == "" {
		t.Fatalf("unexpected path %s scope %s", path, scope)
	}
	if rel, relErr := filepath.Rel(home, path); relErr != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("path %s is not under home %s", path, home)
	}
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

func TestInitScopeRootGlobalFailure(t *testing.T) {
	for _, key := range []string{"HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH"} {
		_ = os.Unsetenv(key)
	}
	root, scope, err := initScopeRoot(true)
	if err != nil {
		return
	}
	if scope != "global" || strings.TrimSpace(root) == "" {
		t.Fatalf("unexpected scope/root %s/%s", scope, root)
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
