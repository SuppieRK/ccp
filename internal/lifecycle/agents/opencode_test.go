package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCodeAdapterPlanVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	a := NewOpenCodeAdapter()
	ctx := Context{ScopeRoot: tmp, HomeDir: filepath.Join(tmp, "home")}
	if a.ID() != "opencode" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(tmp), ".opencode") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(tmp))
	}
	if got := len(a.Plan(ctx)); got != 1 {
		t.Fatalf("plan len=%d want 1", got)
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
	if _, err := a.Uninstall(ctx); err != nil {
		t.Fatalf("second uninstall error: %v", err)
	}
}

func TestOpenCodeVerifyErrorBranchesAndGlobalRoot(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	scope := home
	ctx := Context{ScopeRoot: scope, HomeDir: home}
	if !strings.Contains(opencodeConfigRoot(ctx), filepath.Join(".config", "opencode")) {
		t.Fatalf("expected global opencode root, got %q", opencodeConfigRoot(ctx))
	}

	a := NewOpenCodeAdapter()
	pluginPath := filepath.Join(opencodeConfigRoot(ctx), "plugins", opencodePluginName)
	if err := os.MkdirAll(filepath.Dir(pluginPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pluginPath, []byte("export default {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if a.Verify(ctx) == nil {
		t.Fatal("expected verify failure for invalid plugin content")
	}
}

func TestOpenCodePluginScriptUsesSafeChainRewrite(t *testing.T) {
	script := opencodePluginContent()
	if !strings.Contains(script, `if (/['"\\]|\$\(|\$\{|<</.test(command))`) {
		t.Fatalf("expected conservative complexity fallback guard in OpenCode plugin, got: %s", script)
	}
	if !strings.Contains(script, `command.replace(/(^|\|\||&&|\||;)\s*(?!ccp\b)/g, "$1 ccp ")`) {
		t.Fatalf("expected chained-segment rewrite rule in OpenCode plugin, got: %s", script)
	}
	if !strings.Contains(script, `if (rewritten === command)`) {
		t.Fatalf("expected no-op guard when rewrite does not change command, got: %s", script)
	}
	if !strings.Contains(script, `output.args.command = rewritten;`) {
		t.Fatalf("expected OpenCode plugin to persist rewritten command, got: %s", script)
	}
}
