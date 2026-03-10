package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHubCopilotAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: filepath.Join(tmp, "repo"), HomeDir: home}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewGitHubCopilotAdapter()
	if a.ID() != "github-copilot" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".github") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
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
}
