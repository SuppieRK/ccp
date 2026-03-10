package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAiderConfigHelpersPreserveOtherReadEntries(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, aiderConfigPath)
	readPath := filepath.Join(tmp, aiderRulesPath)
	if err := os.WriteFile(configPath, []byte("read:\n  - CONVENTIONS.md\nmodel: sonnet\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	updated, err := upsertAiderReadConfig(configPath, readPath)
	if err != nil {
		t.Fatalf("upsert aider config: %v", err)
	}
	if !strings.Contains(updated, "- CONVENTIONS.md") || !strings.Contains(updated, "- "+readPath) {
		t.Fatalf("expected both read entries preserved, got: %s", updated)
	}
	if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
		t.Fatalf("write updated aider config: %v", err)
	}

	updated, changed, removeAll, err := removeAiderReadConfig(configPath, readPath)
	if err != nil || !changed || removeAll {
		t.Fatalf("unexpected remove result changed=%v removeAll=%v err=%v", changed, removeAll, err)
	}
	if strings.Contains(updated, readPath) {
		t.Fatalf("expected aider read path removed, got: %s", updated)
	}
	if !strings.Contains(updated, "CONVENTIONS.md") || !strings.Contains(updated, "model: sonnet") {
		t.Fatalf("expected unrelated config preserved, got: %s", updated)
	}
}

func TestAiderAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(scopeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewAiderAdapter()
	if a.ID() != "aider" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".aider.conf.yml") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 2 || !strings.HasSuffix(plan[0].Path, ".aider.conf.yml") || !strings.HasSuffix(plan[1].Path, ".aider.rules.md") {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf(errInstallFmt, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
	res, err := a.Uninstall(ctx)
	if err != nil || res.Applied != 2 {
		t.Fatalf(errUnexpectedUninstFmt, res, err)
	}
}
