package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKiroAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".kiro"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewKiroAdapter()
	if a.ID() != "kiro" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".kiro") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".kiro", "steering", "AGENTS.md")) {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf(errInstallFmt, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
	res, err := a.Uninstall(ctx)
	if err != nil || res.Applied != 1 {
		t.Fatalf(errUnexpectedUninstFmt, res, err)
	}
}

func TestKiroSteeringContentUsesCanonicalGuidance(t *testing.T) {
	content := ccpManagedBlockTemplate()
	if !strings.Contains(content, ccpManagedBlockStart) || !strings.Contains(content, ccpManagedBlockEnd) {
		t.Fatalf("expected managed block markers, got: %s", content)
	}
	if !strings.Contains(content, "Use `ccp` as the command prefix for every executable in shell commands") {
		t.Fatalf("expected canonical ccp guidance, got: %s", content)
	}
}
