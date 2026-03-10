package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	a := NewCodexAdapter()
	ctx := Context{ScopeRoot: filepath.Join(tmp, "repo"), HomeDir: home}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if a.ID() != "codex" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".codex") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
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
