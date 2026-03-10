package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKilocodeAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".kilocode"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewKilocodeAdapter()
	if a.ID() != "kilocode" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".kilocode") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".config", "kilocode", "plugins", opencodePluginName)) || !strings.HasPrefix(plan[0].Path, home) {
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

func TestKilocodePluginContentUsesOpenCodeFamilyHook(t *testing.T) {
	content := opencodePluginContent()
	if !strings.Contains(content, `"tool.execute.before"`) {
		t.Fatalf("expected tool.execute.before hook, got: %s", content)
	}
	if !strings.Contains(content, `input.tool !== "bash"`) {
		t.Fatalf("expected bash-only guard, got: %s", content)
	}
	if !strings.Contains(content, `output.args.command = rewritten`) {
		t.Fatalf("expected command rewrite, got: %s", content)
	}
}
