package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAntigravityAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewAntigravityAdapter()
	if a.ID() != "antigravity" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".agent") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".gemini", "GEMINI.md")) || !strings.HasPrefix(plan[0].Path, home) {
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

func TestAntigravityPlanReusesGeminiFamilyTarget(t *testing.T) {
	tmp := t.TempDir()
	ctx := Context{ScopeRoot: filepath.Join(tmp, "repo"), HomeDir: filepath.Join(tmp, "home")}
	antigravityPlan := NewAntigravityAdapter().Plan(ctx)
	geminiPlan := NewGeminiAdapter().Plan(ctx)
	if len(antigravityPlan) != 1 || len(geminiPlan) != 1 {
		t.Fatalf("unexpected antigravity/gemini plan lengths: antigravity=%d gemini=%d", len(antigravityPlan), len(geminiPlan))
	}
	if antigravityPlan[0].Path != geminiPlan[0].Path {
		t.Fatalf("expected antigravity target %s to match gemini target %s", antigravityPlan[0].Path, geminiPlan[0].Path)
	}
}
