package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCursorAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: filepath.Join(tmp, "home")}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewCursorAdapter()
	if a.ID() != "cursor" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".cursor") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".cursor", "rules", "ccp.mdc")) {
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

func TestCursorRuleContentUsesMinimalAlwaysApplyMetadata(t *testing.T) {
	content := cursorRuleContent()
	if !strings.HasPrefix(content, "---\n") {
		t.Fatalf("expected frontmatter start, got: %s", content)
	}
	if !strings.Contains(content, "description: Route shell commands through ccp") {
		t.Fatalf("expected description metadata, got: %s", content)
	}
	if !strings.Contains(content, "alwaysApply: true") {
		t.Fatalf("expected alwaysApply metadata, got: %s", content)
	}
	if !strings.Contains(content, "Use `ccp` as the command prefix for every executable in shell commands") {
		t.Fatalf("expected canonical ccp guidance, got: %s", content)
	}
	if strings.Contains(content, ccpManagedBlockStart) || strings.Contains(content, ccpManagedBlockEnd) {
		t.Fatalf("did not expect managed block markers in cursor rule, got: %s", content)
	}
}
