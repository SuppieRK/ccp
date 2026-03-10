package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTraeAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: filepath.Join(tmp, "home")}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".trae"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewTraeAdapter()
	if a.ID() != "trae" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".trae") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".trae", "rules", "ccp.md")) {
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

func TestTraeRuleContentUsesCanonicalGuidance(t *testing.T) {
	content := traeRuleContent()
	if !strings.Contains(content, "## CCP Integration (Managed)") {
		t.Fatalf("expected managed heading, got: %s", content)
	}
	if !strings.Contains(content, "Use `ccp` as the command prefix for every executable in shell commands") {
		t.Fatalf("expected canonical ccp guidance, got: %s", content)
	}
	if !strings.Contains(content, ccpRawEscapeHatch) {
		t.Fatalf("expected raw escape hatch note, got: %s", content)
	}
	if strings.Contains(content, "alwaysApply: true") || strings.Contains(content, "trigger: always_on") {
		t.Fatalf("did not expect cursor or windsurf metadata in trae rule, got: %s", content)
	}
	if strings.Contains(content, ccpManagedBlockStart) || strings.Contains(content, ccpManagedBlockEnd) {
		t.Fatalf("did not expect managed block markers in trae rule, got: %s", content)
	}
}
