package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCostrictAliasReusesRooCodeCanonicalTarget(t *testing.T) {
	tmp := t.TempDir()
	ctx := Context{ScopeRoot: filepath.Join(tmp, "repo"), HomeDir: filepath.Join(tmp, "home")}
	adapters := DefaultAdapters()

	aliasPlan := adapters[NormalizeToolID(string(AgentCostrict))].Plan(ctx)
	canonicalPlan := adapters[string(AgentRooCode)].Plan(ctx)
	if len(aliasPlan) != 1 || len(canonicalPlan) != 1 {
		t.Fatalf("unexpected alias/canonical plan lengths: alias=%d canonical=%d", len(aliasPlan), len(canonicalPlan))
	}
	if aliasPlan[0].Path != canonicalPlan[0].Path {
		t.Fatalf("expected alias target %s to match canonical target %s", aliasPlan[0].Path, canonicalPlan[0].Path)
	}
}

func TestRooCodeAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".roo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewRooCodeAdapter()
	if a.ID() != "roocode" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".roo") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".roo", "rules", "ccp.md")) || !strings.HasPrefix(plan[0].Path, home) {
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

func TestRooCodeRuleContentUsesCanonicalGuidance(t *testing.T) {
	content := roocodeRuleContent()
	if !strings.Contains(content, "## CCP Integration (Managed)") {
		t.Fatalf("expected managed heading, got: %s", content)
	}
	if !strings.Contains(content, "Use `ccp` as the command prefix for every executable in shell commands") {
		t.Fatalf("expected canonical ccp guidance, got: %s", content)
	}
	if strings.Contains(content, ccpManagedBlockStart) || strings.Contains(content, ccpManagedBlockEnd) {
		t.Fatalf("did not expect managed block markers in roocode rule, got: %s", content)
	}
}
