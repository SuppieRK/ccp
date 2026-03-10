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

func TestRunUninstallAiderRemovesManagedConfigAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("aider")); err != nil {
		t.Fatalf("aider init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("aider")); err != nil {
		t.Fatalf("aider uninstall failed: %v", err)
	}

	configPath := filepath.Join(home, ".aider.conf.yml")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("expected aider config to be removed, err=%v", err)
	}
	rulesPath := filepath.Join(home, ".aider.rules.md")
	if _, err := os.Stat(rulesPath); !os.IsNotExist(err) {
		t.Fatalf("expected aider rules file to be removed, err=%v", err)
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

func TestRunUninstallQwenRemovesManagedBlockAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".qwen"), "mkdir .qwen: %v")
	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("qwen")); err != nil {
		t.Fatalf("qwen init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("qwen")); err != nil {
		t.Fatalf("qwen uninstall failed: %v", err)
	}

	agentsPath := filepath.Join(home, ".qwen", "AGENTS.md")
	if _, err := os.Stat(agentsPath); !os.IsNotExist(err) {
		t.Fatalf("expected qwen agents file to be removed, err=%v", err)
	}
	settingsPath := filepath.Join(home, ".qwen", "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("expected qwen settings file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallQoderRemovesManagedBlockAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".qoder"), "mkdir .qoder: %v")
	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("qoder")); err != nil {
		t.Fatalf("qoder init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("qoder")); err != nil {
		t.Fatalf("qoder uninstall failed: %v", err)
	}

	agentsPath := filepath.Join(home, ".qoder", "AGENTS.md")
	if _, err := os.Stat(agentsPath); !os.IsNotExist(err) {
		t.Fatalf("expected qoder agents file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallFactoryRemovesManagedHomeBlockAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".factory"), "mkdir .factory: %v")
	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("factory")); err != nil {
		t.Fatalf("factory init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("factory")); err != nil {
		t.Fatalf("factory uninstall failed: %v", err)
	}

	agentsPath := filepath.Join(home, ".factory", "AGENTS.md")
	if _, err := os.Stat(agentsPath); !os.IsNotExist(err) {
		t.Fatalf("expected factory agents file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallAuggieRemovesManagedBlockAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".augment"), "mkdir .augment: %v")
	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("auggie")); err != nil {
		t.Fatalf("auggie init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("auggie")); err != nil {
		t.Fatalf("auggie uninstall failed: %v", err)
	}

	agentsPath := filepath.Join(tmp, "AGENTS.md")
	if _, err := os.Stat(agentsPath); !os.IsNotExist(err) {
		t.Fatalf("expected auggie agents file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallCodeBuddyRemovesManagedSettingsHookAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".codebuddy"), "mkdir .codebuddy: %v")
	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("codebuddy")); err != nil {
		t.Fatalf("codebuddy init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("codebuddy")); err != nil {
		t.Fatalf("codebuddy uninstall failed: %v", err)
	}

	hookPath := filepath.Join(home, ".codebuddy", "hooks", "ccp-rewrite.sh")
	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Fatalf("expected codebuddy hook script to be removed, err=%v", err)
	}
	settingsPath := filepath.Join(home, ".codebuddy", "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("expected codebuddy settings file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallCrushRemovesManagedContextAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".crush"), "mkdir .crush: %v")
	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("crush")); err != nil {
		t.Fatalf("crush init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("crush")); err != nil {
		t.Fatalf("crush uninstall failed: %v", err)
	}

	contextPath := filepath.Join(home, ".config", "crush", "CRUSH.md")
	if _, err := os.Stat(contextPath); !os.IsNotExist(err) {
		t.Fatalf("expected crush context file to be removed, err=%v", err)
	}
	configPath := filepath.Join(home, ".config", "crush", "crush.json")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("expected crush config file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallIFlowRemovesManagedHomeBlockAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".iflow"), "mkdir .iflow: %v")
	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("iflow")); err != nil {
		t.Fatalf("iflow init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("iflow")); err != nil {
		t.Fatalf("iflow uninstall failed: %v", err)
	}

	memoryPath := filepath.Join(home, ".iflow", "IFLOW.md")
	if _, err := os.Stat(memoryPath); !os.IsNotExist(err) {
		t.Fatalf("expected iflow memory file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallPiRemovesManagedBlockAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".pi"), "mkdir .pi: %v")
	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("pi")); err != nil {
		t.Fatalf("pi init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("pi")); err != nil {
		t.Fatalf("pi uninstall failed: %v", err)
	}

	agentsPath := filepath.Join(tmp, "AGENTS.md")
	if _, err := os.Stat(agentsPath); !os.IsNotExist(err) {
		t.Fatalf("expected pi agents file to be removed, err=%v", err)
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

func TestRunUninstallAntigravityRemovesManagedGeminiFamilyTargetAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".agent"), "mkdir .agent: %v")

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("antigravity")); err != nil {
		t.Fatalf("antigravity init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("antigravity")); err != nil {
		t.Fatalf("antigravity uninstall failed: %v", err)
	}

	instructionsPath := filepath.Join(home, ".gemini", "GEMINI.md")
	if _, err := os.Stat(instructionsPath); !os.IsNotExist(err) {
		t.Fatalf("expected antigravity gemini-family instructions to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallWindsurfRemovesManagedHooksAndInitConfig(t *testing.T) {
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

	hookPath := filepath.Join(home, ".codeium", "windsurf", "hooks", "ccp-block.sh")
	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Fatalf("expected windsurf hook script to be removed, err=%v", err)
	}
	hooksPath := filepath.Join(home, ".codeium", "windsurf", "hooks.json")
	if _, err := os.Stat(hooksPath); !os.IsNotExist(err) {
		t.Fatalf("expected windsurf hooks file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallClineRemovesManagedHookAndInitConfig(t *testing.T) {
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

	hookPath := filepath.Join(home, "Documents", "Cline", "Hooks", "PreToolUse")
	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Fatalf("expected cline hook file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallContinueRemovesManagedSettingsHookAndInitConfig(t *testing.T) {
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

	hookPath := filepath.Join(home, ".continue", "hooks", "ccp-rewrite.sh")
	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Fatalf("expected continue hook script to be removed, err=%v", err)
	}
	settingsPath := filepath.Join(home, ".continue", "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("expected continue settings file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallContinuePreservesUnrelatedSettings(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".continue"), "mkdir .continue: %v")
	mkdirAllForTest(t, filepath.Join(home, ".continue"), "mkdir global .continue: %v")

	settingsPath := filepath.Join(home, ".continue", "settings.json")
	hookPath := filepath.Join(home, ".continue", "hooks", "ccp-rewrite.sh")
	escapedHook := strings.ReplaceAll(hookPath, "\\", "\\\\")
	settings := "{\n  \"model\": \"gpt-5\",\n  \"hooks\": {\n    \"PreToolUse\": [\n      {\n        \"matcher\": \"Bash\",\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"" + escapedHook + "\"\n          }\n        ]\n      }\n    ]\n  }\n}\n"
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		t.Fatalf("mkdir continue hooks: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(settings), 0o644); err != nil {
		t.Fatalf("write continue settings: %v", err)
	}
	if err := os.WriteFile(hookPath, []byte("#!/usr/bin/env sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write continue hook: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunUninstall(toolsArgs("continue")); err != nil {
		t.Fatalf("continue uninstall failed: %v", err)
	}

	b, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read continue settings after uninstall: %v", err)
	}
	if !strings.Contains(string(b), `"model": "gpt-5"`) {
		t.Fatalf("expected unrelated continue settings preserved, got: %s", string(b))
	}
	if strings.Contains(string(b), escapedHook) {
		t.Fatalf("expected managed continue hook removed, got: %s", string(b))
	}
}

func TestRunUninstallKiroRemovesManagedSteeringAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".kiro"), "mkdir .kiro: %v")

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("kiro")); err != nil {
		t.Fatalf("kiro init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("kiro")); err != nil {
		t.Fatalf("kiro uninstall failed: %v", err)
	}

	rulePath := filepath.Join(home, ".kiro", "steering", "AGENTS.md")
	if _, err := os.Stat(rulePath); !os.IsNotExist(err) {
		t.Fatalf("expected kiro steering file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallKilocodeRemovesManagedPluginAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".kilocode"), "mkdir .kilocode: %v")

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("kilocode")); err != nil {
		t.Fatalf("kilocode init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("kilocode")); err != nil {
		t.Fatalf("kilocode uninstall failed: %v", err)
	}

	pluginPath := filepath.Join(home, ".config", "kilocode", "plugins", initOpenCodeRewriteJS)
	if _, err := os.Stat(pluginPath); !os.IsNotExist(err) {
		t.Fatalf("expected kilocode plugin file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallRooCodeRemovesManagedRuleAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".roo"), "mkdir .roo: %v")

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("roocode")); err != nil {
		t.Fatalf("roocode init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("roocode")); err != nil {
		t.Fatalf("roocode uninstall failed: %v", err)
	}

	rulePath := filepath.Join(home, ".roo", "rules", "ccp.md")
	if _, err := os.Stat(rulePath); !os.IsNotExist(err) {
		t.Fatalf("expected roocode rule file to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallCostrictAliasRemovesManagedRuleAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".roo"), "mkdir .roo: %v")

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("costrict")); err != nil {
		t.Fatalf("costrict init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("costrict")); err != nil {
		t.Fatalf("costrict uninstall failed: %v", err)
	}

	rulePath := filepath.Join(home, ".roo", "rules", "ccp.md")
	if _, err := os.Stat(rulePath); !os.IsNotExist(err) {
		t.Fatalf("expected roocode rule file to be removed via costrict alias, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "ccp", initConfigName)); !os.IsNotExist(err) {
		t.Fatalf("expected managed init config to be removed after uninstall, err=%v", err)
	}
}

func TestRunUninstallTraeRemovesManagedRuleAndInitConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".trae"), "mkdir .trae: %v")

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("trae")); err != nil {
		t.Fatalf("trae init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("trae")); err != nil {
		t.Fatalf("trae uninstall failed: %v", err)
	}

	rulePath := filepath.Join(tmp, ".trae", "rules", "ccp.md")
	if _, err := os.Stat(rulePath); !os.IsNotExist(err) {
		t.Fatalf("expected trae rule file to be removed, err=%v", err)
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

func TestRunUninstallAiderPreservesOtherConfigEntries(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	chdirForTest(t, tmp)
	configPath := filepath.Join(home, ".aider.conf.yml")
	if err := os.WriteFile(configPath, []byte("read:\n  - CONVENTIONS.md\nmodel: sonnet\n"), 0o644); err != nil {
		t.Fatalf("write aider config: %v", err)
	}
	rulesPath := filepath.Join(home, ".aider.rules.md")
	if err := os.WriteFile(rulesPath, []byte("# User Notes\n\n"+`<!-- BEGIN: CCP MANAGED BLOCK -->`+"\nmanaged content\n"+`<!-- END: CCP MANAGED BLOCK -->`+"\n"), 0o644); err != nil {
		t.Fatalf("write aider rules: %v", err)
	}

	if err := RunInit(toolsArgs("aider")); err != nil {
		t.Fatalf("aider init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("aider")); err != nil {
		t.Fatalf("aider uninstall failed: %v", err)
	}

	b, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read aider config after uninstall: %v", err)
	}
	got := string(b)
	if strings.Contains(got, ".aider.rules.md") {
		t.Fatalf("expected aider rules path removed, got: %s", got)
	}
	if !strings.Contains(got, "CONVENTIONS.md") || !strings.Contains(got, "model: sonnet") {
		t.Fatalf("expected unrelated config preserved, got: %s", got)
	}
	rulesBytes, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("read aider rules after uninstall: %v", err)
	}
	if strings.Contains(string(rulesBytes), "managed content") {
		t.Fatalf("expected managed aider rules removed, got: %s", string(rulesBytes))
	}
	if !strings.Contains(string(rulesBytes), "# User Notes") {
		t.Fatalf("expected unrelated aider rules content preserved, got: %s", string(rulesBytes))
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

func TestRunUninstallQwenPreservesNonCCPContent(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".qwen"), "mkdir .qwen: %v")

	agentsPath := filepath.Join(home, ".qwen", "AGENTS.md")
	mkdirAllForTest(t, filepath.Dir(agentsPath), "mkdir ~/.qwen: %v")
	content := "# User Content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nmanaged content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"
	if err := os.WriteFile(agentsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write qwen agents file: %v", err)
	}
	settingsPath := filepath.Join(home, ".qwen", "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{\n  \"theme\": \"light\",\n  \"context\": {\n    \"fileName\": \"AGENTS.md\"\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write qwen settings file: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunUninstall(toolsArgs("qwen")); err != nil {
		t.Fatalf("qwen uninstall failed: %v", err)
	}

	b, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read agents after uninstall: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "managed content") {
		t.Fatalf("expected managed content removed, got: %s", got)
	}
	if !strings.Contains(got, "# User Content") || !strings.Contains(got, "# Tail") {
		t.Fatalf("expected non-managed content preserved, got: %s", got)
	}
	settingsBytes, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after uninstall: %v", err)
	}
	if !strings.Contains(string(settingsBytes), `"theme": "light"`) {
		t.Fatalf("expected unrelated qwen settings preserved, got: %s", string(settingsBytes))
	}
	if strings.Contains(string(settingsBytes), `"fileName": "AGENTS.md"`) {
		t.Fatalf("expected managed qwen context.fileName removed, got: %s", string(settingsBytes))
	}
}

func TestRunUninstallQoderPreservesNonCCPContent(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".qoder"), "mkdir .qoder: %v")

	agentsPath := filepath.Join(home, ".qoder", "AGENTS.md")
	mkdirAllForTest(t, filepath.Dir(agentsPath), "mkdir ~/.qoder: %v")
	content := "# User Content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nmanaged content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"
	if err := os.WriteFile(agentsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write qoder agents file: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunUninstall(toolsArgs("qoder")); err != nil {
		t.Fatalf("qoder uninstall failed: %v", err)
	}

	b, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read agents after uninstall: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "managed content") {
		t.Fatalf("expected managed content removed, got: %s", got)
	}
	if !strings.Contains(got, "# User Content") || !strings.Contains(got, "# Tail") {
		t.Fatalf("expected non-managed content preserved, got: %s", got)
	}
}

func TestRunUninstallFactoryPreservesHomeNonCCPContent(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".factory"), "mkdir .factory: %v")
	mkdirAllForTest(t, filepath.Join(home, ".factory"), "mkdir home .factory: %v")

	agentsPath := filepath.Join(home, ".factory", "AGENTS.md")
	content := "# User Content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nmanaged content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"
	if err := os.WriteFile(agentsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write factory agents file: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunUninstall(toolsArgs("factory")); err != nil {
		t.Fatalf("factory uninstall failed: %v", err)
	}

	b, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read agents after uninstall: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "managed content") {
		t.Fatalf("expected managed content removed, got: %s", got)
	}
	if !strings.Contains(got, "# User Content") || !strings.Contains(got, "# Tail") {
		t.Fatalf("expected non-managed content preserved, got: %s", got)
	}
}

func TestRunUninstallAuggiePreservesNonCCPContent(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".augment"), "mkdir .augment: %v")

	agentsPath := filepath.Join(tmp, "AGENTS.md")
	content := "# User Content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nmanaged content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"
	if err := os.WriteFile(agentsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write auggie agents file: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunUninstall(toolsArgs("auggie")); err != nil {
		t.Fatalf("auggie uninstall failed: %v", err)
	}

	b, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read agents after uninstall: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "managed content") {
		t.Fatalf("expected managed content removed, got: %s", got)
	}
	if !strings.Contains(got, "# User Content") || !strings.Contains(got, "# Tail") {
		t.Fatalf("expected non-managed content preserved, got: %s", got)
	}
}

func TestRunUninstallCrushPreservesNonCCPContent(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".crush"), "mkdir .crush: %v")
	mkdirAllForTest(t, filepath.Join(home, ".config", "crush"), "mkdir crush config: %v")

	agentsPath := filepath.Join(home, ".config", "crush", "CRUSH.md")
	content := "# User Content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nmanaged content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"
	if err := os.WriteFile(agentsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write crush agents file: %v", err)
	}
	configPath := filepath.Join(home, ".config", "crush", "crush.json")
	contextRef := strings.ReplaceAll(agentsPath, "\\", "\\\\")
	config := "{\n  \"theme\": \"dark\",\n  \"options\": {\n    \"context_paths\": [\n      \"" + contextRef + "\"\n    ]\n  }\n}\n"
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write crush config file: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunUninstall(toolsArgs("crush")); err != nil {
		t.Fatalf("crush uninstall failed: %v", err)
	}

	b, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read agents after uninstall: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "managed content") {
		t.Fatalf("expected managed content removed, got: %s", got)
	}
	if !strings.Contains(got, "# User Content") || !strings.Contains(got, "# Tail") {
		t.Fatalf("expected non-managed content preserved, got: %s", got)
	}
	cfg, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read crush config after uninstall: %v", err)
	}
	if !strings.Contains(string(cfg), `"theme": "dark"`) {
		t.Fatalf("expected unrelated crush config preserved, got: %s", string(cfg))
	}
	if strings.Contains(string(cfg), contextRef) {
		t.Fatalf("expected managed crush context path removed, got: %s", string(cfg))
	}
}

func TestRunUninstallIFlowPreservesHomeNonCCPContent(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".iflow"), "mkdir .iflow: %v")
	mkdirAllForTest(t, filepath.Join(home, ".iflow"), "mkdir home .iflow: %v")

	memoryPath := filepath.Join(home, ".iflow", "IFLOW.md")
	content := "# User Content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nmanaged content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"
	if err := os.WriteFile(memoryPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write iflow memory file: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunUninstall(toolsArgs("iflow")); err != nil {
		t.Fatalf("iflow uninstall failed: %v", err)
	}

	b, err := os.ReadFile(memoryPath)
	if err != nil {
		t.Fatalf("read memory after uninstall: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "managed content") {
		t.Fatalf("expected managed content removed, got: %s", got)
	}
	if !strings.Contains(got, "# User Content") || !strings.Contains(got, "# Tail") {
		t.Fatalf("expected non-managed content preserved, got: %s", got)
	}
}

func TestRunUninstallPiPreservesNonCCPContent(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".pi"), "mkdir .pi: %v")
	chdirForTest(t, tmp)

	agentsPath := filepath.Join(tmp, "AGENTS.md")
	initial := "team notes\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nmanaged content\n<!-- END: CCP MANAGED BLOCK -->\n"
	if err := os.WriteFile(agentsPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write pi agents file: %v", err)
	}

	if err := RunUninstall(toolsArgs("pi")); err != nil {
		t.Fatalf("pi uninstall failed: %v", err)
	}

	got, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read pi agents file: %v", err)
	}
	if strings.TrimSpace(string(got)) != "team notes" {
		t.Fatalf("unexpected preserved content: %q", string(got))
	}
}

func TestRunUninstallCodeBuddyPreservesUnrelatedSettings(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".codebuddy"), "mkdir .codebuddy: %v")
	mkdirAllForTest(t, filepath.Join(home, ".codebuddy"), "mkdir home .codebuddy: %v")

	settingsPath := filepath.Join(home, ".codebuddy", "settings.json")
	hookPath := filepath.Join(home, ".codebuddy", "hooks", "ccp-rewrite.sh")
	escapedHook := strings.ReplaceAll(hookPath, "\\", "\\\\")
	settings := "{\n  \"theme\": \"light\",\n  \"hooks\": {\n    \"PreToolUse\": [\n      {\n        \"matcher\": \"Bash\",\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"" + escapedHook + "\"\n          }\n        ]\n      }\n    ]\n  }\n}\n"
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		t.Fatalf("mkdir codebuddy hooks: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(settings), 0o644); err != nil {
		t.Fatalf("write codebuddy settings: %v", err)
	}
	if err := os.WriteFile(hookPath, []byte("#!/usr/bin/env sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write codebuddy hook: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunUninstall(toolsArgs("codebuddy")); err != nil {
		t.Fatalf("codebuddy uninstall failed: %v", err)
	}

	b, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read codebuddy settings after uninstall: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"theme": "light"`) {
		t.Fatalf("expected unrelated codebuddy settings preserved, got: %s", got)
	}
	if strings.Contains(got, escapedHook) {
		t.Fatalf("expected managed codebuddy hook removed, got: %s", got)
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

func TestRunUninstallAntigravityPreservesOtherGeminiFamilyContent(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(home, ".gemini"), "mkdir home .gemini: %v")
	instructionsPath := filepath.Join(home, ".gemini", "GEMINI.md")
	initial := "# User Header\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nmanaged content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"
	if err := os.WriteFile(instructionsPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write gemini-family instructions: %v", err)
	}

	chdirForTest(t, tmp)
	if err := RunUninstall(toolsArgs("antigravity")); err != nil {
		t.Fatalf("antigravity uninstall failed: %v", err)
	}
	got, err := os.ReadFile(instructionsPath)
	if err != nil {
		t.Fatalf("read gemini-family instructions after uninstall: %v", err)
	}
	s := string(got)
	if strings.Contains(s, "managed content") {
		t.Fatalf("expected managed content removed, got: %s", s)
	}
	if !strings.Contains(s, "# User Header") || !strings.Contains(s, "# Tail") {
		t.Fatalf("expected non-managed gemini-family content preserved, got: %s", s)
	}
}

func TestRunUninstallWindsurfPreservesOtherWindsurfFilesAndDirectories(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	hooksDir := filepath.Join(home, ".codeium", "windsurf", "hooks")
	mkdirAllForTest(t, hooksDir, "mkdir windsurf hooks: %v")
	otherHook := filepath.Join(hooksDir, "other.sh")
	if err := os.WriteFile(otherHook, []byte("#!/usr/bin/env sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write other hook: %v", err)
	}
	hooksPath := filepath.Join(home, ".codeium", "windsurf", "hooks.json")
	otherConfig := "{\n  \"pre_run_command\": [\n    {\n      \"name\": \"other\",\n      \"command\": \"" + strings.ReplaceAll(otherHook, "\\", "\\\\") + "\",\n      \"enabled\": true\n    }\n  ]\n}\n"
	if err := os.WriteFile(hooksPath, []byte(otherConfig), 0o644); err != nil {
		t.Fatalf("write hooks config: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("windsurf")); err != nil {
		t.Fatalf("windsurf init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("windsurf")); err != nil {
		t.Fatalf("windsurf uninstall failed: %v", err)
	}

	if _, err := os.Stat(otherHook); err != nil {
		t.Fatalf("expected other windsurf hook preserved, err=%v", err)
	}
	b, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("read hooks config after uninstall: %v", err)
	}
	if !strings.Contains(string(b), "other") {
		t.Fatalf("expected unrelated windsurf hook config preserved, got: %s", string(b))
	}
	if st, err := os.Stat(hooksDir); err != nil || !st.IsDir() {
		t.Fatalf("expected windsurf hooks directory preserved, err=%v", err)
	}
}

func TestRunUninstallClinePreservesOtherClineFilesAndDirectories(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	hooksDir := filepath.Join(home, "Documents", "Cline", "Hooks")
	mkdirAllForTest(t, hooksDir, "mkdir cline hooks: %v")
	otherHook := filepath.Join(hooksDir, "PostToolUse")
	if err := os.WriteFile(otherHook, []byte("#!/usr/bin/env sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write other hook: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("cline")); err != nil {
		t.Fatalf("cline init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("cline")); err != nil {
		t.Fatalf("cline uninstall failed: %v", err)
	}

	if _, err := os.Stat(otherHook); err != nil {
		t.Fatalf("expected other cline hook preserved, err=%v", err)
	}
	if st, err := os.Stat(hooksDir); err != nil || !st.IsDir() {
		t.Fatalf("expected cline hooks directory preserved, err=%v", err)
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

func TestRunUninstallTraePreservesOtherTraeFilesAndDirectories(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".trae", "rules"), "mkdir .trae/rules: %v")
	otherRule := filepath.Join(tmp, ".trae", "rules", "team.md")
	if err := os.WriteFile(otherRule, []byte("team rule\n"), 0o644); err != nil {
		t.Fatalf("write other rule: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("trae")); err != nil {
		t.Fatalf("trae init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("trae")); err != nil {
		t.Fatalf("trae uninstall failed: %v", err)
	}

	if _, err := os.Stat(otherRule); err != nil {
		t.Fatalf("expected other trae rule preserved, err=%v", err)
	}
	if st, err := os.Stat(filepath.Join(tmp, ".trae", "rules")); err != nil || !st.IsDir() {
		t.Fatalf("expected .trae/rules directory preserved, err=%v", err)
	}
}

func TestRunUninstallKiroPreservesOtherKiroFilesAndDirectories(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(home, ".kiro", "steering"), "mkdir ~/.kiro/steering: %v")
	otherRule := filepath.Join(home, ".kiro", "steering", "team.md")
	if err := os.WriteFile(otherRule, []byte("team rule\n"), 0o644); err != nil {
		t.Fatalf("write other rule: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("kiro")); err != nil {
		t.Fatalf("kiro init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("kiro")); err != nil {
		t.Fatalf("kiro uninstall failed: %v", err)
	}

	if _, err := os.Stat(otherRule); err != nil {
		t.Fatalf("expected other kiro steering preserved, err=%v", err)
	}
	if st, err := os.Stat(filepath.Join(home, ".kiro", "steering")); err != nil || !st.IsDir() {
		t.Fatalf("expected ~/.kiro/steering directory preserved, err=%v", err)
	}
}

func TestRunUninstallKilocodePreservesOtherHomePluginFilesAndDirectories(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(home, ".config", "kilocode", "plugins"), "mkdir kilocode plugins: %v")
	otherPlugin := filepath.Join(home, ".config", "kilocode", "plugins", "team.js")
	if err := os.WriteFile(otherPlugin, []byte("export default {}\n"), 0o644); err != nil {
		t.Fatalf("write other plugin: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("kilocode")); err != nil {
		t.Fatalf("kilocode init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("kilocode")); err != nil {
		t.Fatalf("kilocode uninstall failed: %v", err)
	}

	if _, err := os.Stat(otherPlugin); err != nil {
		t.Fatalf("expected other kilocode plugin preserved, err=%v", err)
	}
	if st, err := os.Stat(filepath.Join(home, ".config", "kilocode", "plugins")); err != nil || !st.IsDir() {
		t.Fatalf("expected kilocode plugins directory preserved, err=%v", err)
	}
}

func TestRunUninstallRooCodePreservesOtherRooCodeFilesAndDirectories(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, "mkdir home: %v")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(home, ".roo", "rules"), "mkdir ~/.roo/rules: %v")
	otherRule := filepath.Join(home, ".roo", "rules", "team.md")
	if err := os.WriteFile(otherRule, []byte("team rule\n"), 0o644); err != nil {
		t.Fatalf("write other rule: %v", err)
	}

	chdirForTest(t, tmp)

	if err := RunInit(toolsArgs("roocode")); err != nil {
		t.Fatalf("roocode init failed: %v", err)
	}
	if err := RunUninstall(toolsArgs("roocode")); err != nil {
		t.Fatalf("roocode uninstall failed: %v", err)
	}

	if _, err := os.Stat(otherRule); err != nil {
		t.Fatalf("expected other roocode rule preserved, err=%v", err)
	}
	if st, err := os.Stat(filepath.Join(home, ".roo", "rules")); err != nil || !st.IsDir() {
		t.Fatalf("expected ~/.roo/rules directory preserved, err=%v", err)
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
	home := filepath.Join(tmp, "home")
	setHomeDirForTest(t, home)
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
	if _, err := os.Stat(filepath.Join(home, ".codeium", "windsurf", "hooks", "ccp-block.sh")); !os.IsNotExist(err) {
		t.Fatalf("expected windsurf hook removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codeium", "windsurf", "hooks.json")); !os.IsNotExist(err) {
		t.Fatalf("expected windsurf hooks config removed, err=%v", err)
	}
}

func TestRunUninstallDetectsRooCodeWhenToolsFlagOmitted(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)
	home := filepath.Join(tmp, "home")
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, ".roo"), "mkdir .roo: %v")

	if err := RunInit(toolsArgs("roocode")); err != nil {
		t.Fatalf("init roocode: %v", err)
	}
	if err := os.Remove(filepath.Join(home, ".config", "ccp", initConfigName)); err != nil {
		t.Fatalf("remove init config: %v", err)
	}
	if err := RunUninstall(nil); err != nil {
		t.Fatalf("uninstall auto-detect: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".roo", "rules", "ccp.md")); !os.IsNotExist(err) {
		t.Fatalf("expected roocode rule removed, err=%v", err)
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
