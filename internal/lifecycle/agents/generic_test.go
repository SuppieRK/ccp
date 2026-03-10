package agents

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallPlannedArtifactsCounts(t *testing.T) {
	tmp := t.TempDir()
	plan := []PlannedArtifact{
		{Kind: ArtifactHook, Path: filepath.Join(tmp, "hook.sh"), Content: "echo hook", Perm: 0o750},
		{Kind: ArtifactSettings, Path: filepath.Join(tmp, "settings.conf"), Content: "ok", Perm: 0o644},
	}
	res, err := InstallPlannedArtifacts(plan, writeFileWriter)
	if err != nil {
		t.Fatalf("install error: %v", err)
	}
	if res.Applied != 2 || res.Noop != 0 {
		t.Fatalf("unexpected result %+v", res)
	}
}

func TestResolveHomeScopedPath(t *testing.T) {
	if got := ResolveHomeScopedPath("", "rel"); got != "rel" {
		t.Fatalf("expected rel when home empty, got %s", got)
	}
	home := filepath.Join(t.TempDir(), "home")
	got := ResolveHomeScopedPath(home, "bin")
	rel, err := filepath.Rel(home, got)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("expected joined path under home, got %s (home=%s)", got, home)
	}
}

func TestResolveRepoScopedPath(t *testing.T) {
	if got := ResolveRepoScopedPath("", "rel"); got != "rel" {
		t.Fatalf("expected rel when scope root empty, got %s", got)
	}
	root := filepath.Join(t.TempDir(), "repo")
	got := ResolveRepoScopedPath(root, "rules/ccp.md")
	rel, err := filepath.Rel(root, got)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("expected joined path under repo root, got %s (root=%s)", got, root)
	}
}

func TestGenericAdapterPlanInstallVerify(t *testing.T) {
	tmp := t.TempDir()
	a := NewGenericAdapter("alpha", ".alpha")
	ctx := Context{ScopeRoot: tmp, HomeDir: tmp}
	if a.ID() != "alpha" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(tmp), ".alpha") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(tmp))
	}
	if got := len(a.Plan(ctx)); got != 3 {
		t.Fatalf("plan len=%d want 3", got)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf(errInstallFmt, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
}
