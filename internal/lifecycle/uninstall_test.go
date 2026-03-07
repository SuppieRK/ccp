package lifecycle

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/lifecycle/agents"
)

const (
	toolsFlag      = "--tools"
	initConfigName = "init.json"
	claudeDirName  = ".claude"
	claudeHookName = "ccp-rewrite.sh"
)

func toolsArgs(tool string) []string {
	return []string{toolsFlag, tool}
}

func TestRunUninstallCodexRemovesManagedBlockAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, "mkdir work: %v")
	chdirForTest(t, work)

	if err := RunInit(toolsArgs("codex")); err != nil {
		t.Fatalf("codex init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("codex")); err != nil {
		t.Fatalf("codex uninstall failed: %v", err)
	}

	agentsPath := filepath.Join(home, ".codex", "AGENTS.md")
	if _, err := os.Stat(agentsPath); !os.IsNotExist(err) {
		t.Fatalf("expected codex agents file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallGitHubCopilotRemovesManagedBlockAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, "mkdir work: %v")
	chdirForTest(t, work)

	if err := RunInit(toolsArgs("github-copilot")); err != nil {
		t.Fatalf("github copilot init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("github-copilot")); err != nil {
		t.Fatalf("github copilot uninstall failed: %v", err)
	}

	instructionsPath := filepath.Join(home, ".copilot", "copilot-instructions.md")
	if _, err := os.Stat(instructionsPath); !os.IsNotExist(err) {
		t.Fatalf("expected github copilot instructions file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallGeminiRemovesManagedBlockAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, "mkdir work: %v")
	chdirForTest(t, work)

	if err := RunInit(toolsArgs("gemini")); err != nil {
		t.Fatalf("gemini init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("gemini")); err != nil {
		t.Fatalf("gemini uninstall failed: %v", err)
	}

	instructionsPath := filepath.Join(home, ".gemini", "GEMINI.md")
	if _, err := os.Stat(instructionsPath); !os.IsNotExist(err) {
		t.Fatalf("expected gemini instructions file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallCursorRemovesManagedRuleAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".cursor"), "mkdir .cursor: %v")

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("cursor")); err != nil {
		t.Fatalf("cursor init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("cursor")); err != nil {
		t.Fatalf("cursor uninstall failed: %v", err)
	}

	rulePath := filepath.Join(tmp, ".cursor", "rules", "ccp.mdc")
	if _, err := os.Stat(rulePath); !os.IsNotExist(err) {
		t.Fatalf("expected cursor rule file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallAmazonQRemovesManagedRuleAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".amazonq"), "mkdir .amazonq: %v")

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("amazon-q")); err != nil {
		t.Fatalf("amazon q init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("amazon-q")); err != nil {
		t.Fatalf("amazon q uninstall failed: %v", err)
	}

	rulePath := filepath.Join(tmp, ".amazonq", "rules", "ccp.md")
	if _, err := os.Stat(rulePath); !os.IsNotExist(err) {
		t.Fatalf("expected amazon q rule file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallWindsurfRemovesManagedRuleAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".windsurf"), "mkdir .windsurf: %v")

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("windsurf")); err != nil {
		t.Fatalf("windsurf init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("windsurf")); err != nil {
		t.Fatalf("windsurf uninstall failed: %v", err)
	}

	rulePath := filepath.Join(tmp, ".windsurf", "rules", "ccp.md")
	if _, err := os.Stat(rulePath); !os.IsNotExist(err) {
		t.Fatalf("expected windsurf rule file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallClineRemovesManagedRuleAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".clinerules"), "mkdir .clinerules: %v")

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("cline")); err != nil {
		t.Fatalf("cline init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("cline")); err != nil {
		t.Fatalf("cline uninstall failed: %v", err)
	}

	rulePath := filepath.Join(tmp, ".clinerules", "ccp.md")
	if _, err := os.Stat(rulePath); !os.IsNotExist(err) {
		t.Fatalf("expected cline rule file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallContinueRemovesManagedRuleAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".continue"), "mkdir .continue: %v")

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("continue")); err != nil {
		t.Fatalf("continue init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("continue")); err != nil {
		t.Fatalf("continue uninstall failed: %v", err)
	}

	rulePath := filepath.Join(tmp, ".continue", "rules", "ccp.md")
	if _, err := os.Stat(rulePath); !os.IsNotExist(err) {
		t.Fatalf("expected continue rule file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallOpenCodeRemovesPluginAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)
	home := filepath.Join(tmp, "home")
	setHomeDirForTest(t, home)

	if err := RunInit(toolsArgs("opencode")); err != nil {
		t.Fatalf("opencode init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("opencode")); err != nil {
		t.Fatalf("opencode uninstall failed: %v", err)
	}

	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "ccp-rewrite.js")
	if _, err := os.Stat(pluginPath); !os.IsNotExist(err) {
		t.Fatalf("expected opencode plugin to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallClaudeRemovesHookSettingsAndAwareness(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, "mkdir work: %v")
	chdirForTest(t, work)

	if err := RunInit(toolsArgs("claude")); err != nil {
		t.Fatalf("claude init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("claude")); err != nil {
		t.Fatalf("claude uninstall failed: %v", err)
	}

	for _, p := range []string{
		filepath.Join(home, claudeDirName, "hooks", claudeHookName),
		filepath.Join(home, claudeDirName, "settings.json"),
		filepath.Join(home, claudeDirName, "CCP.md"),
	} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("expected %s removed, err=%v", p, err)
		}
	}
}

func TestRunUninstallClaudePreservesNonCCPHooks(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, filepath.Join(home, claudeDirName, "hooks"), "mkdir .claude/hooks: %v")
	setHomeDirForTest(t, home)

	settingsPath := filepath.Join(home, claudeDirName, "settings.json")
	hookPath := filepath.Join(home, claudeDirName, "hooks", claudeHookName)
	otherPath := filepath.Join(home, claudeDirName, "hooks", "other.sh")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write ccp hook: %v", err)
	}
	if err := os.WriteFile(otherPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write other hook: %v", err)
	}
	settings := "{\n  \"hooks\": {\n    \"PreToolUse\": [\n      {\n        \"matcher\": \"Bash\",\n        \"hooks\": [\n          {\"type\": \"command\", \"command\": \"" + strings.ReplaceAll(hookPath, "\\", "\\\\") + "\"}\n        ]\n      },\n      {\n        \"matcher\": \"Bash\",\n        \"hooks\": [\n          {\"type\": \"command\", \"command\": \"" + strings.ReplaceAll(otherPath, "\\", "\\\\") + "\"}\n        ]\n      }\n    ]\n  }\n}\n"
	if err := os.WriteFile(settingsPath, []byte(settings), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, "mkdir work: %v")
	chdirForTest(t, work)

	if err := RunUninstall(toolsArgs("claude")); err != nil {
		t.Fatalf("claude uninstall failed: %v", err)
	}

	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Fatalf("expected ccp hook to be removed, err=%v", err)
	}
	b, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after uninstall: %v", err)
	}
	got := string(b)
	if strings.Contains(got, claudeHookName) {
		t.Fatalf("expected ccp hook entry removed, got: %s", got)
	}
	if !strings.Contains(got, "other.sh") {
		t.Fatalf("expected non-ccp hook preserved, got: %s", got)
	}
}

func TestRunUninstallGitHubCopilotPreservesNonCCPContent(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, filepath.Join(home, ".copilot"), "mkdir .copilot: %v")
	setHomeDirForTest(t, home)

	instructionsPath := filepath.Join(home, ".copilot", "copilot-instructions.md")
	content := "# User Content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nmanaged content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"
	if err := os.WriteFile(instructionsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write github copilot instructions: %v", err)
	}

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, "mkdir work: %v")
	chdirForTest(t, work)

	if err := RunUninstall(toolsArgs("github-copilot")); err != nil {
		t.Fatalf("github copilot uninstall failed: %v", err)
	}

	b, err := os.ReadFile(instructionsPath)
	if err != nil {
		t.Fatalf("read instructions after uninstall: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "managed content") {
		t.Fatalf("expected managed content removed, got: %s", got)
	}
	if !strings.Contains(got, "# User Content") || !strings.Contains(got, "# Tail") {
		t.Fatalf("expected non-managed content preserved, got: %s", got)
	}
}

func TestRunUninstallGeminiPreservesNonCCPContent(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, filepath.Join(home, ".gemini"), "mkdir .gemini: %v")
	setHomeDirForTest(t, home)

	instructionsPath := filepath.Join(home, ".gemini", "GEMINI.md")
	content := "# User Content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nmanaged content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"
	if err := os.WriteFile(instructionsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write gemini instructions: %v", err)
	}

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, "mkdir work: %v")
	chdirForTest(t, work)

	if err := RunUninstall(toolsArgs("gemini")); err != nil {
		t.Fatalf("gemini uninstall failed: %v", err)
	}

	b, err := os.ReadFile(instructionsPath)
	if err != nil {
		t.Fatalf("read instructions after uninstall: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "managed content") {
		t.Fatalf("expected managed content removed, got: %s", got)
	}
	if !strings.Contains(got, "# User Content") || !strings.Contains(got, "# Tail") {
		t.Fatalf("expected non-managed content preserved, got: %s", got)
	}
}

func TestRunUninstallCursorPreservesOtherCursorFilesAndDirectories(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".cursor", "rules"), "mkdir .cursor/rules: %v")
	otherRule := filepath.Join(tmp, ".cursor", "rules", "team.mdc")
	if err := os.WriteFile(otherRule, []byte("team rule\n"), 0o644); err != nil {
		t.Fatalf("write other rule: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("cursor")); err != nil {
		t.Fatalf("cursor init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("cursor")); err != nil {
		t.Fatalf("cursor uninstall failed: %v", err)
	}

	if _, err := os.Stat(otherRule); err != nil {
		t.Fatalf("expected other cursor rule preserved, err=%v", err)
	}
	if st, err := os.Stat(filepath.Join(tmp, ".cursor", "rules")); err != nil || !st.IsDir() {
		t.Fatalf("expected .cursor/rules directory preserved, err=%v", err)
	}
}

func TestRunUninstallAmazonQPreservesOtherAmazonQFilesAndDirectories(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".amazonq", "rules"), "mkdir .amazonq/rules: %v")
	otherRule := filepath.Join(tmp, ".amazonq", "rules", "team.md")
	if err := os.WriteFile(otherRule, []byte("team rule\n"), 0o644); err != nil {
		t.Fatalf("write other rule: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("amazon-q")); err != nil {
		t.Fatalf("amazon q init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("amazon-q")); err != nil {
		t.Fatalf("amazon q uninstall failed: %v", err)
	}

	if _, err := os.Stat(otherRule); err != nil {
		t.Fatalf("expected other amazon q rule preserved, err=%v", err)
	}
	if st, err := os.Stat(filepath.Join(tmp, ".amazonq", "rules")); err != nil || !st.IsDir() {
		t.Fatalf("expected .amazonq/rules directory preserved, err=%v", err)
	}
}

func TestRunUninstallWindsurfPreservesOtherWindsurfFilesAndDirectories(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".windsurf", "rules"), "mkdir .windsurf/rules: %v")
	otherRule := filepath.Join(tmp, ".windsurf", "rules", "team.md")
	if err := os.WriteFile(otherRule, []byte("team rule\n"), 0o644); err != nil {
		t.Fatalf("write other rule: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("windsurf")); err != nil {
		t.Fatalf("windsurf init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("windsurf")); err != nil {
		t.Fatalf("windsurf uninstall failed: %v", err)
	}

	if _, err := os.Stat(otherRule); err != nil {
		t.Fatalf("expected other windsurf rule preserved, err=%v", err)
	}
	if st, err := os.Stat(filepath.Join(tmp, ".windsurf", "rules")); err != nil || !st.IsDir() {
		t.Fatalf("expected .windsurf/rules directory preserved, err=%v", err)
	}
}

func TestRunUninstallClinePreservesOtherClineFilesAndDirectories(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".clinerules"), "mkdir .clinerules: %v")
	otherRule := filepath.Join(tmp, ".clinerules", "team.md")
	if err := os.WriteFile(otherRule, []byte("team rule\n"), 0o644); err != nil {
		t.Fatalf("write other rule: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("cline")); err != nil {
		t.Fatalf("cline init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("cline")); err != nil {
		t.Fatalf("cline uninstall failed: %v", err)
	}

	if _, err := os.Stat(otherRule); err != nil {
		t.Fatalf("expected other cline rule preserved, err=%v", err)
	}
	if st, err := os.Stat(filepath.Join(tmp, ".clinerules")); err != nil || !st.IsDir() {
		t.Fatalf("expected .clinerules directory preserved, err=%v", err)
	}
}

func TestRunUninstallContinuePreservesOtherContinueFilesAndDirectories(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".continue", "rules"), "mkdir .continue/rules: %v")
	otherRule := filepath.Join(tmp, ".continue", "rules", "team.md")
	if err := os.WriteFile(otherRule, []byte("team rule\n"), 0o644); err != nil {
		t.Fatalf("write other rule: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("continue")); err != nil {
		t.Fatalf("continue init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("continue")); err != nil {
		t.Fatalf("continue uninstall failed: %v", err)
	}

	if _, err := os.Stat(otherRule); err != nil {
		t.Fatalf("expected other continue rule preserved, err=%v", err)
	}
	if st, err := os.Stat(filepath.Join(tmp, ".continue", "rules")); err != nil || !st.IsDir() {
		t.Fatalf("expected .continue/rules directory preserved, err=%v", err)
	}
}

type uninstallStubAdapter struct {
	id string
}

func (a uninstallStubAdapter) ID() string { return a.id }

func (a uninstallStubAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, a.id)
}

func (a uninstallStubAdapter) Install(_ agents.Context, _ agents.WriterFunc) (agents.InstallResult, error) {
	return agents.InstallResult{}, nil
}

func (a uninstallStubAdapter) Plan(_ agents.Context) []agents.PlannedArtifact {
	return nil
}

func (a uninstallStubAdapter) Verify(_ agents.Context) error {
	return nil
}

type uninstallCapableAdapter struct {
	uninstallStubAdapter
	res agents.InstallResult
	err error
}

func (a uninstallCapableAdapter) Uninstall(_ agents.Context) (agents.InstallResult, error) {
	return a.res, a.err
}

func TestApplyUninstallAdaptersReportsNoopRemovedAndError(t *testing.T) {
	ctx := agents.Context{ScopeRoot: t.TempDir(), HomeDir: t.TempDir()}
	adapters := map[string]agents.Adapter{
		"noop":  uninstallStubAdapter{id: "noop"},
		"gone":  uninstallCapableAdapter{uninstallStubAdapter: uninstallStubAdapter{id: "gone"}, res: agents.InstallResult{Applied: 1}},
		"error": uninstallCapableAdapter{uninstallStubAdapter: uninstallStubAdapter{id: "error"}, err: errors.New("boom")},
	}

	states, err := applyUninstallAdapters(ctx, []string{"noop", "gone"}, adapters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(states) != 2 || states[0].Status != "noop" || states[1].Status != "removed" {
		t.Fatalf("unexpected states: %+v", states)
	}

	states, err = applyUninstallAdapters(ctx, []string{"error"}, adapters)
	if err == nil {
		t.Fatal("expected error")
	}
	if len(states) != 1 || states[0].Status != "failed" {
		t.Fatalf("unexpected error state: %+v", states)
	}
}

func TestLoadConfiguredToolsAndJoinTools(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, initConfigName)

	cfg := initConfig{Tools: []string{"codex", "claude"}}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	tools, err := loadConfiguredTools(path)
	if err != nil {
		t.Fatalf("loadConfiguredTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("unexpected tools %v", tools)
	}

	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfiguredTools(path); err == nil {
		t.Fatal("expected invalid init config error")
	}

	adapters := map[string]agents.Adapter{
		"beta":  uninstallStubAdapter{id: "beta"},
		"alpha": uninstallStubAdapter{id: "alpha"},
	}
	if got := joinTools(adapters); got != "alpha, beta" {
		t.Fatalf("unexpected joined tools %q", got)
	}
}

func TestUpdateInitConfigAfterUninstallUpdatesAndRemoves(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)
	setHomeDirForTest(t, tmp)

	path, err := initPath()
	if err != nil {
		t.Fatal(err)
	}
	mkdirAllForTest(t, filepath.Dir(path), "mkdir config dir: %v")
	cfg := initConfig{
		Tools: []string{"claude", "codex"},
		State: []toolState{{Tool: "claude", Status: "applied"}, {Tool: "codex", Status: "applied"}},
	}
	initial, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	initial = append(initial, '\n')
	if err := os.WriteFile(path, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := updateInitConfigAfterUninstall([]string{"claude"}); err != nil {
		t.Fatalf("update config: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "claude") || !strings.Contains(string(got), "codex") {
		t.Fatalf("unexpected config content: %s", string(got))
	}
	matches, err := filepath.Glob(path + ".bak.*")
	if err != nil || len(matches) == 0 {
		t.Fatalf("expected backup file, matches=%v err=%v", matches, err)
	}

	if err := updateInitConfigAfterUninstall([]string{"codex"}); err != nil {
		t.Fatalf("remove final tool: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected config removed, err=%v", err)
	}
}

func TestRunUninstallDetectsToolWhenToolsFlagOmitted(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)
	setHomeDirForTest(t, filepath.Join(tmp, "home"))

	if err := RunInit(toolsArgs("opencode")); err != nil {
		t.Fatalf("init opencode: %v", err)
	}
	if err := os.Remove(filepath.Join(tmp, "home", ".config", "ccp", initConfigName)); err != nil {
		t.Fatalf("remove init config: %v", err)
	}
	mkdirAllForTest(t, filepath.Join(tmp, ".opencode"), "mkdir .opencode: %v")
	if err := RunUninstall(nil); err != nil {
		t.Fatalf("uninstall auto-detect: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "home", ".config", "opencode", "plugins", "ccp-rewrite.js")); !os.IsNotExist(err) {
		t.Fatalf("expected plugin removed, err=%v", err)
	}
}

func TestRunUninstallDetectsGitHubCopilotWhenToolsFlagOmitted(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)
	setHomeDirForTest(t, filepath.Join(tmp, "home"))

	if err := RunInit(toolsArgs("github-copilot")); err != nil {
		t.Fatalf("init github copilot: %v", err)
	}
	if err := os.Remove(filepath.Join(tmp, "home", ".config", "ccp", initConfigName)); err != nil {
		t.Fatalf("remove init config: %v", err)
	}
	mkdirAllForTest(t, filepath.Join(tmp, ".github"), "mkdir .github: %v")
	if err := RunUninstall(nil); err != nil {
		t.Fatalf("uninstall auto-detect: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "home", ".copilot", "copilot-instructions.md")); !os.IsNotExist(err) {
		t.Fatalf("expected github copilot instructions removed, err=%v", err)
	}
}

func TestRunUninstallDetectsGeminiWhenToolsFlagOmitted(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)
	setHomeDirForTest(t, filepath.Join(tmp, "home"))

	if err := RunInit(toolsArgs("gemini")); err != nil {
		t.Fatalf("init gemini: %v", err)
	}
	if err := os.Remove(filepath.Join(tmp, "home", ".config", "ccp", initConfigName)); err != nil {
		t.Fatalf("remove init config: %v", err)
	}
	mkdirAllForTest(t, filepath.Join(tmp, ".gemini"), "mkdir .gemini: %v")
	if err := RunUninstall(nil); err != nil {
		t.Fatalf("uninstall auto-detect: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "home", ".gemini", "GEMINI.md")); !os.IsNotExist(err) {
		t.Fatalf("expected gemini instructions removed, err=%v", err)
	}
}

func TestRunUninstallDetectsCursorWhenToolsFlagOmitted(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)
	setHomeDirForTest(t, filepath.Join(tmp, "home"))
	mkdirAllForTest(t, filepath.Join(tmp, ".cursor"), "mkdir .cursor: %v")

	if err := RunInit(toolsArgs("cursor")); err != nil {
		t.Fatalf("init cursor: %v", err)
	}
	if err := os.Remove(filepath.Join(tmp, "home", ".config", "ccp", initConfigName)); err != nil {
		t.Fatalf("remove init config: %v", err)
	}
	if err := RunUninstall(nil); err != nil {
		t.Fatalf("uninstall auto-detect: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".cursor", "rules", "ccp.mdc")); !os.IsNotExist(err) {
		t.Fatalf("expected cursor rule removed, err=%v", err)
	}
}

func TestRunUninstallDetectsAmazonQWhenToolsFlagOmitted(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)
	setHomeDirForTest(t, filepath.Join(tmp, "home"))
	mkdirAllForTest(t, filepath.Join(tmp, ".amazonq"), "mkdir .amazonq: %v")

	if err := RunInit(toolsArgs("amazon-q")); err != nil {
		t.Fatalf("init amazon q: %v", err)
	}
	if err := os.Remove(filepath.Join(tmp, "home", ".config", "ccp", initConfigName)); err != nil {
		t.Fatalf("remove init config: %v", err)
	}
	if err := RunUninstall(nil); err != nil {
		t.Fatalf("uninstall auto-detect: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".amazonq", "rules", "ccp.md")); !os.IsNotExist(err) {
		t.Fatalf("expected amazon q rule removed, err=%v", err)
	}
}

func TestRunUninstallDetectsWindsurfWhenToolsFlagOmitted(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)
	setHomeDirForTest(t, filepath.Join(tmp, "home"))
	mkdirAllForTest(t, filepath.Join(tmp, ".windsurf"), "mkdir .windsurf: %v")

	if err := RunInit(toolsArgs("windsurf")); err != nil {
		t.Fatalf("init windsurf: %v", err)
	}
	if err := os.Remove(filepath.Join(tmp, "home", ".config", "ccp", initConfigName)); err != nil {
		t.Fatalf("remove init config: %v", err)
	}
	if err := RunUninstall(nil); err != nil {
		t.Fatalf("uninstall auto-detect: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".windsurf", "rules", "ccp.md")); !os.IsNotExist(err) {
		t.Fatalf("expected windsurf rule removed, err=%v", err)
	}
}

func TestRunUninstallReturnsErrorWhenNoToolsDetected(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)
	setHomeDirForTest(t, filepath.Join(tmp, "home"))

	if err := RunUninstall(nil); err == nil || !strings.Contains(err.Error(), "no tools detected") {
		t.Fatalf("expected no tools detected error, got %v", err)
	}
}

func TestRunUninstallUsesConfiguredToolsWhenFlagOmitted(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)
	setHomeDirForTest(t, filepath.Join(tmp, "home"))

	if err := RunInit(toolsArgs("opencode")); err != nil {
		t.Fatalf("init opencode: %v", err)
	}
	if err := RunUninstall(nil); err != nil {
		t.Fatalf("uninstall using configured tools: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "home", ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected init config removed, err=%v", err)
	}
}
