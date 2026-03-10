package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindsurfHooksConfigHelpersUpsertAndVerify(t *testing.T) {
	tmp := t.TempDir()
	hooksPath := filepath.Join(tmp, "hooks.json")
	managedHook := filepath.Join(tmp, "ccp-block.sh")

	updated, err := upsertWindsurfHooksConfig(hooksPath, managedHook)
	if err != nil {
		t.Fatalf("upsertWindsurfHooksConfig: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(updated), &root); err != nil {
		t.Fatalf("unmarshal updated windsurf hooks: %v", err)
	}
	if !windsurfHookEntriesContain(normalizeWindsurfHookEntries(root["pre_run_command"]), managedHook) {
		t.Fatalf("expected managed hook path in config, got: %s", updated)
	}
	if err := os.WriteFile(hooksPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err := windsurfHooksConfigHasEntry(hooksPath, managedHook)
	if err != nil || !ok {
		t.Fatalf("expected windsurf config to contain managed hook, ok=%v err=%v", ok, err)
	}
}

func TestWindsurfHooksConfigHelpersRemoveBranches(t *testing.T) {
	tmp := t.TempDir()
	hooksPath := filepath.Join(tmp, "hooks.json")
	managedHook := filepath.Join(tmp, "ccp-block.sh")
	otherHook := filepath.Join(tmp, "other.sh")

	content := "{\n  \"pre_run_command\": [\n    {\n      \"name\": \"ccp-pre-run-command\",\n      \"command\": \"" + strings.ReplaceAll(managedHook, "\\", "\\\\") + "\",\n      \"enabled\": true\n    },\n    {\n      \"name\": \"other\",\n      \"command\": \"" + strings.ReplaceAll(otherHook, "\\", "\\\\") + "\",\n      \"enabled\": true\n    }\n  ]\n}\n"
	if err := os.WriteFile(hooksPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, changed, removeAll, err := removeWindsurfHooksConfig(hooksPath, managedHook)
	if err != nil || !changed || removeAll {
		t.Fatalf("unexpected removeWindsurfHooksConfig result changed=%v removeAll=%v err=%v", changed, removeAll, err)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(removed), &root); err != nil {
		t.Fatalf("unmarshal removed windsurf hooks: %v", err)
	}
	entries := normalizeWindsurfHookEntries(root["pre_run_command"])
	if windsurfHookEntriesContain(entries, managedHook) || !windsurfHookEntriesContain(entries, otherHook) {
		t.Fatalf("expected managed hook removed and other hook preserved, got: %s", removed)
	}

	if err := os.WriteFile(hooksPath, []byte("{\n  \"pre_run_command\": [\n    {\n      \"name\": \"ccp-pre-run-command\",\n      \"command\": \""+strings.ReplaceAll(managedHook, "\\", "\\\\")+"\",\n      \"enabled\": true\n    }\n  ]\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, changed, removeAll, err = removeWindsurfHooksConfig(hooksPath, managedHook)
	if err != nil || !changed || !removeAll {
		t.Fatalf("expected remove-all windsurf branch, changed=%v removeAll=%v err=%v", changed, removeAll, err)
	}
}

func TestWindsurfHookEntriesFallback(t *testing.T) {
	if got := normalizeWindsurfHookEntries("unexpected"); got != nil {
		t.Fatalf("expected nil windsurf entries for non-slice input, got=%v", got)
	}
}

func TestWindsurfAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".windsurf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewWindsurfAdapter()
	if a.ID() != "windsurf" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".windsurf") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 2 || !strings.HasSuffix(plan[0].Path, filepath.Join(".codeium", "windsurf", "hooks", "ccp-block.sh")) || !strings.HasSuffix(plan[1].Path, filepath.Join(".codeium", "windsurf", "hooks.json")) {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf(errInstallFmt, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
	res, err := a.Uninstall(ctx)
	if err != nil || res.Applied != 2 {
		t.Fatalf(errUnexpectedUninstFmt, res, err)
	}
}

func TestWindsurfHookScriptExecutesExpectedBranches(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		withCCP      bool
		wantLog      string
		wantExitCode int
		wantStderr   string
	}{
		{name: "missing ccp", input: `{"command":"pwd"}`, wantLog: "skip-no-ccp"},
		{name: "empty input", input: "", withCCP: true, wantLog: "skip-empty-input"},
		{name: "missing command", input: `{"tool_input":{}}`, withCCP: true, wantLog: "skip-no-command"},
		{name: "already prefixed", input: `{"command":"ccp pwd"}`, withCCP: true, wantLog: "skip-already-prefixed"},
		{name: "block", input: `{"command":"pwd"}`, withCCP: true, wantExitCode: 2, wantStderr: "Retry as: ccp pwd"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := runHookScript(t, windsurfHookScriptName, "ccp-windsurf-hook.log", windsurfHookScriptContent(), tc.input, tc.withCCP)
			if result.exitCode != tc.wantExitCode {
				t.Fatalf("expected windsurf exit code %d, got %d stdout=%q stderr=%q", tc.wantExitCode, result.exitCode, result.stdout, result.stderr)
			}
			if tc.wantLog != "" && !strings.Contains(result.log, tc.wantLog) {
				t.Fatalf("expected log marker %q, got %q", tc.wantLog, result.log)
			}
			if tc.wantStderr != "" && !strings.Contains(result.stderr, tc.wantStderr) {
				t.Fatalf("expected stderr to contain %q, got %q", tc.wantStderr, result.stderr)
			}
		})
	}
}

func TestWindsurfRuleContentUsesAlwaysOnMetadata(t *testing.T) {
	content := windsurfHookScriptContent()
	for _, needle := range []string{
		"generated by ccp init for windsurf",
		"pre_run_command hook",
		"Use ccp as the command prefix for shell commands",
		"exit 2",
		`LOG_FILE="${TMPDIR:-/tmp}/ccp-windsurf-hook.log"`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("expected canonical windsurf hook content %q, got: %s", needle, content)
		}
	}
}
