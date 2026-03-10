package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeAdapterPlanInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: filepath.Join(tmp, "repo"), HomeDir: home}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewClaudeAdapter()
	if a.ID() != "claude" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".claude") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	if got := len(a.Plan(ctx)); got != 4 {
		t.Fatalf("plan len=%d want 4", got)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf(errInstallFmt, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
	guidePath := filepath.Join(home, ".claude", claudeGuideName)
	guide, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("read claude guide: %v", err)
	}
	if !strings.Contains(string(guide), "@CCP.md") {
		t.Fatalf("expected claude guide reference in %q", string(guide))
	}
	res, err := a.Uninstall(ctx)
	if err != nil {
		t.Fatalf("uninstall error: %v", err)
	}
	if res.Applied == 0 {
		t.Fatalf("expected uninstall to remove artifacts, got %+v", res)
	}
}

func TestClaudeHookRemovalHelpers(t *testing.T) {
	tmp := t.TempDir()
	settings := filepath.Join(tmp, "settings.json")
	hook := filepath.Join(tmp, "ccp-rewrite.sh")
	if err := os.WriteFile(settings, []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"`+strings.ReplaceAll(hook, "\\", "\\\\")+`"}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := removeClaudePreToolUseHook(settings, hook)
	if err != nil || !changed {
		t.Fatalf("expected changed=true err=nil, got changed=%v err=%v", changed, err)
	}
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		t.Fatalf("expected settings removed when empty, err=%v", err)
	}
	if _, err := removeFileIfExists(filepath.Join(tmp, "missing")); err != nil {
		t.Fatalf("removeFileIfExists missing: %v", err)
	}
}

func TestClaudeGuideBlockHelpers(t *testing.T) {
	tmp := t.TempDir()
	guide := filepath.Join(tmp, claudeGuideName)
	existing := "# Team rules\n\nBe deliberate.\n"

	assertClaudeGuideUpsertIntoMissingFile(t, guide)
	assertClaudeGuideUpsertPreservesExistingContent(t, guide, existing)
	assertClaudeGuideRemovalPreservesUnrelatedContent(t, guide, existing)
	assertClaudeGuideRemovalCanDeleteFullyManagedFile(t, guide)
}

func assertClaudeGuideUpsertIntoMissingFile(t *testing.T, guide string) {
	t.Helper()
	updated, err := upsertClaudeGuideBlock(guide)
	if err != nil {
		t.Fatalf("upsert missing guide: %v", err)
	}
	if !strings.Contains(updated, "@CCP.md") {
		t.Fatalf("expected claude guide reference, got %q", updated)
	}
}

func assertClaudeGuideUpsertPreservesExistingContent(t *testing.T, guide, existing string) {
	t.Helper()
	if err := os.WriteFile(guide, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	updated, err := upsertClaudeGuideBlock(guide)
	if err != nil {
		t.Fatalf("upsert existing guide: %v", err)
	}
	if !strings.Contains(updated, existing) {
		t.Fatalf("expected existing content preserved, got %q", updated)
	}
	if strings.Count(updated, ccpManagedBlockStart) != 1 {
		t.Fatalf("expected single managed block, got %q", updated)
	}
}

func assertClaudeGuideRemovalPreservesUnrelatedContent(t *testing.T, guide, existing string) {
	t.Helper()
	if err := os.WriteFile(guide, []byte(existing+claudeManagedGuideBlock()), 0o644); err != nil {
		t.Fatal(err)
	}
	out, changed, removeAll, err := removeClaudeGuideBlock(guide)
	if err != nil || !changed || removeAll {
		t.Fatalf("unexpected remove result changed=%v removeAll=%v err=%v", changed, removeAll, err)
	}
	if strings.Contains(out, "@CCP.md") {
		t.Fatalf("expected managed claude guide removed, got %q", out)
	}
	if !strings.Contains(out, "Be deliberate.") {
		t.Fatalf("expected unrelated guide content preserved, got %q", out)
	}
}

func assertClaudeGuideRemovalCanDeleteFullyManagedFile(t *testing.T, guide string) {
	t.Helper()
	if err := os.WriteFile(guide, []byte(claudeManagedGuideBlock()), 0o644); err != nil {
		t.Fatal(err)
	}
	_, changed, removeAll, err := removeClaudeGuideBlock(guide)
	if err != nil || !changed || !removeAll {
		t.Fatalf("expected remove-all branch, changed=%v removeAll=%v err=%v", changed, removeAll, err)
	}
}

func TestClaudeAdapterInstallPreservesExistingGuideContent(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: filepath.Join(tmp, "repo"), HomeDir: home}
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	guidePath := filepath.Join(home, ".claude", claudeGuideName)
	original := "# Global Claude Rules\n\nPrefer concise answers.\n"
	if err := os.WriteFile(guidePath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewClaudeAdapter()
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf("install error: %v", err)
	}

	guide, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("read guide: %v", err)
	}
	content := string(guide)
	if !strings.Contains(content, original) {
		t.Fatalf("expected original guide content preserved, got %q", content)
	}
	if !strings.Contains(content, "@CCP.md") {
		t.Fatalf("expected CCP reference in guide, got %q", content)
	}
	if strings.Count(content, ccpManagedBlockStart) != 1 {
		t.Fatalf("expected exactly one managed block, got %q", content)
	}
}

func TestClaudeHookScriptContainsExpectedRuntimeGuards(t *testing.T) {
	script := claudeHookScriptContent()
	if strings.Contains(script, "command -v jq") {
		t.Fatalf("did not expect jq dependency in hook script, got: %s", script)
	}
	if !strings.Contains(script, `command -v ccp`) {
		t.Fatalf("expected ccp availability guard in hook script, got: %s", script)
	}
	if !strings.Contains(script, `LOG_FILE="${TMPDIR:-/tmp}/ccp-claude-hook.log"`) {
		t.Fatalf("expected tmp-folder log target in hook script, got: %s", script)
	}
	if strings.Contains(script, `skip-complex-shape`) {
		t.Fatalf("did not expect heuristic complexity skip marker in hook script, got: %s", script)
	}
	for _, reason := range []string{
		"skip-no-ccp",
		"skip-empty-input",
		"skip-no-command",
		"skip-empty-rewrite",
		"skip-no-change",
		"skip-invalid-shell",
	} {
		if !strings.Contains(script, reason) {
			t.Fatalf("expected troubleshooting marker %q in hook script, got: %s", reason, script)
		}
	}
	if !strings.Contains(script, `REWRITTEN_CMD="$(rewrite_command "$CMD")"`) {
		t.Fatalf("expected shell-native rewrite helper usage, got: %s", script)
	}
	if !strings.Contains(script, `ESCAPED_CMD="$(json_escape "$REWRITTEN_CMD")"`) {
		t.Fatalf("expected shell-native json escaping path, got: %s", script)
	}
	if !strings.Contains(script, `updatedInput\":{\"command\":\"$ESCAPED_CMD\"}`) {
		t.Fatalf("expected updated input payload to use escaped rewritten command, got: %s", script)
	}
}

func TestClaudeHookScriptExecutesExpectedBranches(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		withCCP      bool
		wantLog      string
		wantCommand  string
		wantNoOutput bool
	}{
		{name: "missing ccp dependency", input: `{"tool_input":{"command":"pwd && ls"}}`, wantLog: "skip-no-ccp", wantNoOutput: true},
		{name: "empty input", input: "", withCCP: true, wantLog: "skip-empty-input", wantNoOutput: true},
		{name: "missing command field", input: `{"tool_input":{}}`, withCCP: true, wantLog: "skip-no-command", wantNoOutput: true},
		{name: "empty rewrite from whitespace command", input: `{"tool_input":{"command":"   "}}`, withCCP: true, wantLog: "skip-empty-rewrite", wantNoOutput: true},
		{name: "already prefixed command", input: `{"tool_input":{"command":"ccp pwd"}}`, withCCP: true, wantLog: "skip-no-change", wantNoOutput: true},
		{name: "invalid rewritten shell", input: `{"tool_input":{"command":"pwd &&"}}`, withCCP: true, wantLog: "skip-invalid-shell", wantNoOutput: true},
		{name: "simple chained rewrite", input: `{"tool_input":{"command":"pwd && ls"}}`, withCCP: true, wantCommand: "ccp pwd && ccp ls"},
		{name: "double quoted rewrite", input: `{"tool_input":{"command":"echo \"test\""}}`, withCCP: true, wantCommand: `ccp echo "test"`},
		{name: "single quoted rewrite", input: `{"tool_input":{"command":"echo 'test'"}}`, withCCP: true, wantCommand: `ccp echo 'test'`},
		{name: "backslash command rewrite", input: `{"tool_input":{"command":"echo foo\\ bar"}}`, withCCP: true, wantCommand: `ccp echo foo\ bar`},
		{name: "command substitution rewrite", input: `{"tool_input":{"command":"echo $(pwd)"}}`, withCCP: true, wantCommand: `ccp echo $(pwd)`},
		{name: "parameter expansion rewrite", input: `{"tool_input":{"command":"echo ${HOME}"}}`, withCCP: true, wantCommand: `ccp echo ${HOME}`},
		{name: "heredoc token rewrite", input: `{"tool_input":{"command":"cat <<EOF"}}`, withCCP: true, wantCommand: `ccp cat <<EOF`},
		{name: "quoted chained rewrite", input: `{"tool_input":{"command":"git commit -m \"msg\" && git status"}}`, withCCP: true, wantCommand: `ccp git commit -m "msg" && ccp git status`},
		{name: "mixed prefixed chain rewrite", input: `{"tool_input":{"command":"ccp pwd && ls | env ; whoami || date"}}`, withCCP: true, wantCommand: "ccp pwd && ccp ls | ccp env ; ccp whoami || ccp date"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, logOutput := runClaudeHookScript(t, tc.input, tc.withCCP)
			if !strings.Contains(logOutput, tc.wantLog) && tc.wantLog != "" {
				t.Fatalf("expected log marker %q, got %q", tc.wantLog, logOutput)
			}
			if tc.wantNoOutput {
				if strings.TrimSpace(stdout) != "" {
					t.Fatalf("expected no hook output, got %q", stdout)
				}
				return
			}
			if strings.TrimSpace(logOutput) != "" {
				t.Fatalf("expected no skip log for successful rewrite, got %q", logOutput)
			}
			got := decodeClaudeHookOutput(t, stdout)
			if got != tc.wantCommand {
				t.Fatalf("expected rewritten command %q, got %q", tc.wantCommand, got)
			}
		})
	}
}

func TestClaudeHookRemovalNoChangeBranches(t *testing.T) {
	tmp := t.TempDir()
	settings := filepath.Join(tmp, "settings.json")
	hook := filepath.Join(tmp, "ccp-rewrite.sh")

	if changed, err := removeClaudePreToolUseHook(settings, hook); err != nil || changed {
		t.Fatalf("expected no change when settings missing, changed=%v err=%v", changed, err)
	}
	if err := os.WriteFile(settings, []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := removeClaudePreToolUseHook(settings, hook)
	if err != nil || !changed {
		t.Fatalf("expected removal on invalid json, changed=%v err=%v", changed, err)
	}
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		t.Fatalf("expected settings removed for invalid json, err=%v", err)
	}
}

func runClaudeHookScript(t *testing.T, input string, withCCP bool) (string, string) {
	t.Helper()
	result := runHookScript(t, "ccp-rewrite.sh", "ccp-claude-hook.log", claudeHookScriptContent(), input, withCCP)
	if result.exitCode != 0 {
		t.Fatalf("expected claude hook exit 0, got %d stderr=%q", result.exitCode, result.stderr)
	}
	return result.stdout, result.log
}
