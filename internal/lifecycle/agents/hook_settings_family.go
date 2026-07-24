package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func preToolUseCommandSettingsContent(hookPath string) string {
	escapedHookPath := strings.ReplaceAll(hookPath, "\\", "\\\\")
	return fmt.Sprintf("{\n  \"hooks\": {\n    \"PreToolUse\": [\n      {\n        \"matcher\": \"Bash\",\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"%s\"\n          }\n        ]\n      }\n    ]\n  }\n}\n", escapedHookPath)
}

func bashHookAndSettingsArtifacts(root, hookScriptName, settingsName, hookContent string) []PlannedArtifact {
	hookPath := filepath.Join(root, "hooks", hookScriptName)
	settingsPath := filepath.Join(root, settingsName)
	return []PlannedArtifact{
		{
			Kind:    ArtifactHook,
			Path:    hookPath,
			Content: hookContent,
			Perm:    0o755,
		},
		{
			Kind:    ArtifactSettings,
			Path:    settingsPath,
			Content: preToolUseCommandSettingsContent(hookPath),
			Perm:    0o644,
		},
	}
}

type artifactCheck struct {
	path string
	msg  string
}

func verifyArtifactFiles(checks ...artifactCheck) error {
	for _, check := range checks {
		if _, err := os.Stat(check.path); err != nil {
			return fmt.Errorf(check.msg, check.path)
		}
	}
	return nil
}

func ensureHookArtifactExecutable(path string) error {
	return os.Chmod(path, 0o755)
}

func verifyHookArtifactExecutable(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("hook script is not executable: %s", path)
	}
	return nil
}

func homeOrScopePath(ctx Context, rel ...string) string {
	base := ctx.HomeDir
	if strings.TrimSpace(base) == "" {
		base = ctx.ScopeRoot
	}
	parts := append([]string{base}, rel...)
	return filepath.Join(parts...)
}

func removePreToolUseCommandHook(settingsPath, hookPath string) (bool, error) {
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	root, ok := decodeHookSettings(raw)
	if !ok {
		return false, fmt.Errorf("invalid hook settings file %s: requires manual attention", settingsPath)
	}

	hooks, pre, ok := preToolUseEntries(root)
	if !ok {
		return false, nil
	}

	normalizedHook := filepath.Clean(strings.TrimSpace(hookPath))
	filtered, changed := filterPreToolUseEntries(pre, normalizedHook)
	if !changed {
		return false, nil
	}
	prunePreToolUseHooks(root, hooks, filtered)
	return persistJSONSettings(settingsPath, root)
}

func decodeHookSettings(raw []byte) (map[string]any, bool) {
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return nil, false
	}
	return root, true
}

func preToolUseEntries(root map[string]any) (map[string]any, []any, bool) {
	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		return nil, nil, false
	}
	pre, ok := hooks["PreToolUse"].([]any)
	if !ok || len(pre) == 0 {
		return nil, nil, false
	}
	return hooks, pre, true
}

func filterPreToolUseEntries(pre []any, normalizedHook string) ([]any, bool) {
	filtered := make([]any, 0, len(pre))
	changed := false
	for _, entry := range pre {
		if shouldRemovePreToolUseEntry(entry, normalizedHook) {
			changed = true
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered, changed
}

func shouldRemovePreToolUseEntry(entry any, normalizedHook string) bool {
	m, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	if !isBashMatcher(m) {
		return false
	}
	hookItems, _ := m["hooks"].([]any)
	return hasMatchingCommandHook(hookItems, normalizedHook)
}

func isBashMatcher(entry map[string]any) bool {
	matcher, _ := entry["matcher"].(string)
	return strings.EqualFold(strings.TrimSpace(matcher), "bash")
}

func hasMatchingCommandHook(hookItems []any, normalizedHook string) bool {
	for _, h := range hookItems {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := hm["type"].(string)
		cmd, _ := hm["command"].(string)
		if strings.EqualFold(strings.TrimSpace(typ), "command") && filepath.Clean(strings.TrimSpace(cmd)) == normalizedHook {
			return true
		}
	}
	return false
}

func prunePreToolUseHooks(root, hooks map[string]any, filtered []any) {
	if len(filtered) == 0 {
		delete(hooks, "PreToolUse")
	} else {
		hooks["PreToolUse"] = filtered
	}
	if len(hooks) == 0 {
		delete(root, "hooks")
	}
}

func persistJSONSettings(settingsPath string, root map[string]any) (bool, error) {
	if len(root) == 0 {
		return removeFileIfExists(settingsPath)
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}
	out = append(out, '\n')
	if err := writeManagedArtifact(settingsPath, out, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func hookSettingsUseHook(settingsPath, hookPath, invalidFmt string) (bool, error) {
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		return false, err
	}
	root, ok := decodeHookSettings(raw)
	if !ok {
		return false, fmt.Errorf(invalidFmt, settingsPath)
	}
	_, pre, ok := preToolUseEntries(root)
	if !ok {
		return false, nil
	}
	return preToolUseContains(pre, hookPath), nil
}

func upsertPreToolUseCommandSettings(settingsPath, hookPath, invalidFmt string) (string, error) {
	root := map[string]any{}
	if raw, err := os.ReadFile(settingsPath); err == nil {
		decoded, ok := decodeHookSettings(raw)
		if !ok {
			return "", fmt.Errorf(invalidFmt, settingsPath)
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
	if !preToolUseContains(pre, hookPath) {
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

const (
	codebuddyHookScriptName = "cmdshape-rewrite.sh"
	codebuddySettingsName   = "settings.json"
)

func codebuddyRoot(ctx Context) string {
	return ResolveHomeScopedPath(ctx.HomeDir, ".codebuddy")
}

func codebuddySettingsUseHook(settingsPath, hookPath string) (bool, error) {
	return hookSettingsUseHook(settingsPath, hookPath, "invalid codebuddy settings file: %s")
}

func upsertCodeBuddySettings(settingsPath, hookPath string) (string, error) {
	return upsertPreToolUseCommandSettings(settingsPath, hookPath, "invalid codebuddy settings file: %s")
}

func preToolUseContains(pre []any, hookPath string) bool {
	normalizedHook := filepath.Clean(strings.TrimSpace(hookPath))
	for _, entry := range pre {
		m, ok := entry.(map[string]any)
		if !ok || !isBashMatcher(m) {
			continue
		}
		hooks, _ := m["hooks"].([]any)
		if hasMatchingCommandHook(hooks, normalizedHook) {
			return true
		}
	}
	return false
}

func codebuddyPreToolUseContains(pre []any, hookPath string) bool {
	return preToolUseContains(pre, hookPath)
}
