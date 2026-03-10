package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedHomeRuleFileAdapterSeparatesDetectionFromInstallTarget(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	repo := filepath.Join(tmp, "repo")
	ctx := Context{ScopeRoot: repo, HomeDir: home}
	a := NewManagedHomeRuleFileAdapter(
		"alpha",
		".alpha",
		filepath.Join(".alpha", "rules", "ccp.md"),
		"missing alpha rule file: %s",
		"missing alpha managed guidance in %s",
		func() string { return "ccp-managed\n" },
		[]string{"ccp-managed"},
	)

	if got := a.DetectRoot(repo); got != filepath.Join(repo, ".alpha") {
		t.Fatalf("unexpected detect root %q", got)
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 {
		t.Fatalf("plan len=%d want 1", len(plan))
	}
	if !strings.HasPrefix(plan[0].Path, home) {
		t.Fatalf("expected home-scoped install target, got %s", plan[0].Path)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf("install error: %v", err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf("verify error: %v", err)
	}
}

func TestManagedRepoRuleFileAdapterUninstallPreservesSiblingFiles(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	ctx := Context{ScopeRoot: repo, HomeDir: filepath.Join(tmp, "home")}
	a := NewManagedRepoRuleFileAdapter(
		"alpha",
		".alpha",
		filepath.Join(".alpha", "rules", "ccp.md"),
		"missing alpha rule file: %s",
		"missing alpha managed guidance in %s",
		func() string { return "ccp-managed\n" },
		[]string{"ccp-managed"},
	)
	target := filepath.Join(repo, ".alpha", "rules", "ccp.md")
	sibling := filepath.Join(repo, ".alpha", "rules", "user.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("ccp-managed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sibling, []byte("keep-me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := a.Uninstall(ctx)
	if err != nil || res.Applied != 1 {
		t.Fatalf(errUnexpectedUninstFmt, res, err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected target removed, err=%v", err)
	}
	if got, err := os.ReadFile(sibling); err != nil || string(got) != "keep-me\n" {
		t.Fatalf("expected sibling preserved, got=%q err=%v", string(got), err)
	}
}
