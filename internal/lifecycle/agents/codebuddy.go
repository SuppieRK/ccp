package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	codebuddyHookScriptName = "ccp-rewrite.sh"
	codebuddySettingsName   = "settings.json"
)

type CodeBuddyAdapter struct{}

func NewCodeBuddyAdapter() CodeBuddyAdapter {
	return CodeBuddyAdapter{}
}

func (a CodeBuddyAdapter) ID() string { return string(AgentCodeBuddy) }

func (a CodeBuddyAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".codebuddy")
}

func (a CodeBuddyAdapter) Plan(ctx Context) []PlannedArtifact {
	root := codebuddyRoot(ctx)
	hookPath := filepath.Join(root, "hooks", codebuddyHookScriptName)
	settingsPath := filepath.Join(root, codebuddySettingsName)
	escapedHookPath := strings.ReplaceAll(hookPath, "\\", "\\\\")
	return []PlannedArtifact{
		{
			Kind:    ArtifactHook,
			Path:    hookPath,
			Content: claudeHookScriptContent(),
			Perm:    0o755,
		},
		{
			Kind:    ArtifactSettings,
			Path:    settingsPath,
			Content: fmt.Sprintf("{\n  \"hooks\": {\n    \"PreToolUse\": [\n      {\n        \"matcher\": \"Bash\",\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"%s\"\n          }\n        ]\n      }\n    ]\n  }\n}\n", escapedHookPath),
			Perm:    0o644,
		},
	}
}

func (a CodeBuddyAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	root := codebuddyRoot(ctx)
	hookPath := filepath.Join(root, "hooks", codebuddyHookScriptName)
	settingsPath := filepath.Join(root, codebuddySettingsName)

	var res InstallResult
	hookChanged, err := write(hookPath, []byte(claudeHookScriptContent()), 0o755)
	if err != nil {
		return InstallResult{}, err
	}
	if hookChanged {
		res.Applied++
	} else {
		res.Noop++
	}

	content, err := upsertCodeBuddySettings(settingsPath, hookPath)
	if err != nil {
		return InstallResult{}, err
	}
	settingsChanged, err := write(settingsPath, []byte(content), 0o644)
	if err != nil {
		return InstallResult{}, err
	}
	if settingsChanged {
		res.Applied++
	} else {
		res.Noop++
	}

	return res, nil
}

func (a CodeBuddyAdapter) Verify(ctx Context) error {
	root := codebuddyRoot(ctx)
	hookPath := filepath.Join(root, "hooks", codebuddyHookScriptName)
	settingsPath := filepath.Join(root, codebuddySettingsName)
	for _, check := range []struct {
		path string
		msg  string
	}{
		{path: hookPath, msg: "missing codebuddy hook script: %s"},
		{path: settingsPath, msg: "missing codebuddy settings file: %s"},
	} {
		if _, err := os.Stat(check.path); err != nil {
			return fmt.Errorf(check.msg, check.path)
		}
	}
	ok, err := codebuddySettingsUseHook(settingsPath, hookPath)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("missing codebuddy managed hook contribution in %s", settingsPath)
	}
	return nil
}

func (a CodeBuddyAdapter) Uninstall(ctx Context) (InstallResult, error) {
	root := codebuddyRoot(ctx)
	hookPath := filepath.Join(root, "hooks", codebuddyHookScriptName)
	settingsPath := filepath.Join(root, codebuddySettingsName)

	var res InstallResult
	removed, err := removeFileIfExists(hookPath)
	if err != nil {
		return res, err
	}
	if removed {
		res.Applied++
	}
	changed, err := removeClaudePreToolUseHook(settingsPath, hookPath)
	if err != nil {
		return res, err
	}
	if changed {
		res.Applied++
	} else {
		res.Noop++
	}
	return res, nil
}

func codebuddyRoot(ctx Context) string {
	return ResolveHomeScopedPath(ctx.HomeDir, ".codebuddy")
}

func codebuddySettingsUseHook(settingsPath, hookPath string) (bool, error) {
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		return false, err
	}
	root, ok := decodeClaudeSettings(raw)
	if !ok {
		return false, fmt.Errorf("invalid codebuddy settings file: %s", settingsPath)
	}
	_, pre, ok := claudePreToolUse(root)
	if !ok {
		return false, nil
	}
	for _, entry := range pre {
		m, ok := entry.(map[string]any)
		if !ok || !isClaudeBashMatcher(m) {
			continue
		}
		hooks, _ := m["hooks"].([]any)
		if hasMatchingClaudeCommandHook(hooks, filepath.Clean(strings.TrimSpace(hookPath))) {
			return true, nil
		}
	}
	return false, nil
}

func upsertCodeBuddySettings(settingsPath, hookPath string) (string, error) {
	root := map[string]any{}
	if raw, err := os.ReadFile(settingsPath); err == nil {
		decoded, ok := decodeClaudeSettings(raw)
		if !ok {
			return "", fmt.Errorf("invalid codebuddy settings file: %s", settingsPath)
		}
		root = decoded
	} else if !os.IsNotExist(err) {
		return "", err
	}

	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		root["hooks"] = hooks
	}
	pre, _ := hooks["PreToolUse"].([]any)
	if !codebuddyPreToolUseContains(pre, hookPath) {
		pre = append(pre, map[string]any{
			"matcher": "Bash",
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": hookPath,
				},
			},
		})
	}
	hooks["PreToolUse"] = pre
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	return string(append(out, '\n')), nil
}

func codebuddyPreToolUseContains(pre []any, hookPath string) bool {
	normalizedHook := filepath.Clean(strings.TrimSpace(hookPath))
	for _, entry := range pre {
		m, ok := entry.(map[string]any)
		if !ok || !isClaudeBashMatcher(m) {
			continue
		}
		hooks, _ := m["hooks"].([]any)
		if hasMatchingClaudeCommandHook(hooks, normalizedHook) {
			return true
		}
	}
	return false
}
