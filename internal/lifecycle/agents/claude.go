package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ClaudeAdapter struct{}

const (
	claudeHookScriptName = "ccp-rewrite.sh"
	claudeSettingsName   = "settings.json"
	claudeAwarenessName  = "CCP.md"
)

func NewClaudeAdapter() ClaudeAdapter {
	return ClaudeAdapter{}
}

func (a ClaudeAdapter) ID() string { return "claude" }

func (a ClaudeAdapter) DetectRoot(scopeRoot string) string {
	// Detection remains project-root based for framework auto-selection.
	return filepath.Join(scopeRoot, ".claude")
}

func (a ClaudeAdapter) Plan(ctx Context) []PlannedArtifact {
	root := claudeRoot(ctx)
	hookPath := filepath.Join(root, "hooks", claudeHookScriptName)
	settingsPath := filepath.Join(root, claudeSettingsName)
	awarenessPath := filepath.Join(root, claudeAwarenessName)
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
		{
			Kind:    ArtifactAwareness,
			Path:    awarenessPath,
			Content: awarenessContent(a.ID()),
			Perm:    0o644,
		},
	}
}

func (a ClaudeAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	return InstallPlannedArtifacts(a.Plan(ctx), write)
}

func (a ClaudeAdapter) Verify(ctx Context) error {
	root := claudeRoot(ctx)
	for _, check := range []struct {
		path string
		msg  string
	}{
		{path: filepath.Join(root, "hooks", claudeHookScriptName), msg: "missing hook script: %s"},
		{path: filepath.Join(root, claudeSettingsName), msg: "missing settings file: %s"},
		{path: filepath.Join(root, claudeAwarenessName), msg: "missing awareness file: %s"},
	} {
		if _, err := os.Stat(check.path); err != nil {
			return fmt.Errorf(check.msg, check.path)
		}
	}
	return nil
}

func claudeRoot(ctx Context) string {
	base := ctx.HomeDir
	if strings.TrimSpace(base) == "" {
		base = ctx.ScopeRoot
	}
	return filepath.Join(base, ".claude")
}

func (a ClaudeAdapter) Uninstall(ctx Context) (InstallResult, error) {
	root := claudeRoot(ctx)
	hookPath := filepath.Join(root, "hooks", claudeHookScriptName)
	settingsPath := filepath.Join(root, claudeSettingsName)
	awarenessPath := filepath.Join(root, claudeAwarenessName)

	var res InstallResult
	for _, p := range []string{hookPath, awarenessPath} {
		removed, err := removeFileIfExists(p)
		if err != nil {
			return res, err
		}
		if removed {
			res.Applied++
		}
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

func removeFileIfExists(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func removeClaudePreToolUseHook(settingsPath, hookPath string) (bool, error) {
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	root, ok := decodeClaudeSettings(raw)
	if !ok {
		return removeFileIfExists(settingsPath)
	}

	hooks, pre, ok := claudePreToolUse(root)
	if !ok {
		return false, nil
	}

	normalizedHook := filepath.Clean(strings.TrimSpace(hookPath))
	filtered, changed := filterClaudePreToolUseEntries(pre, normalizedHook)
	if !changed {
		return false, nil
	}
	pruneClaudeHooks(root, hooks, filtered)
	return persistClaudeSettings(settingsPath, root)
}

func decodeClaudeSettings(raw []byte) (map[string]any, bool) {
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return nil, false
	}
	return root, true
}

func claudePreToolUse(root map[string]any) (map[string]any, []any, bool) {
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

func filterClaudePreToolUseEntries(pre []any, normalizedHook string) ([]any, bool) {
	filtered := make([]any, 0, len(pre))
	changed := false
	for _, entry := range pre {
		if shouldRemoveClaudePreToolUseEntry(entry, normalizedHook) {
			changed = true
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered, changed
}

func shouldRemoveClaudePreToolUseEntry(entry any, normalizedHook string) bool {
	m, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	if !isClaudeBashMatcher(m) {
		return false
	}
	hookItems, _ := m["hooks"].([]any)
	return hasMatchingClaudeCommandHook(hookItems, normalizedHook)
}

func isClaudeBashMatcher(entry map[string]any) bool {
	matcher, _ := entry["matcher"].(string)
	return strings.EqualFold(strings.TrimSpace(matcher), "bash")
}

func hasMatchingClaudeCommandHook(hookItems []any, normalizedHook string) bool {
	for _, h := range hookItems {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := hm["type"].(string)
		cmd, _ := hm["command"].(string)
		if strings.EqualFold(strings.TrimSpace(typ), "command") &&
			filepath.Clean(strings.TrimSpace(cmd)) == normalizedHook {
			return true
		}
	}
	return false
}

func pruneClaudeHooks(root, hooks map[string]any, filtered []any) {
	if len(filtered) == 0 {
		delete(hooks, "PreToolUse")
	} else {
		hooks["PreToolUse"] = filtered
	}
	if len(hooks) == 0 {
		delete(root, "hooks")
	}
}

func persistClaudeSettings(settingsPath string, root map[string]any) (bool, error) {
	if len(root) == 0 {
		return removeFileIfExists(settingsPath)
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}
	out = append(out, '\n')
	return true, os.WriteFile(settingsPath, out, 0o644)
}

func claudeHookScriptContent() string {
	return `#!/usr/bin/env sh
# generated by ccp init for claude
# Claude PreToolUse hook: read hook JSON from stdin and optionally rewrite Bash command.

if ! command -v jq >/dev/null 2>&1 || ! command -v ccp >/dev/null 2>&1; then
  exit 0
fi

INPUT="$(cat)"
if [ -z "$INPUT" ]; then
  exit 0
fi

CMD="$(printf '%s' "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null)"
if [ -z "$CMD" ]; then
  exit 0
fi

case "$CMD" in
  ccp\ *|*/ccp\ *) exit 0 ;;
esac

UPDATED_INPUT="$(
  printf '%s' "$INPUT" | jq -c --arg cmd "ccp $CMD" '
    (.tool_input // {}) as $ti
    | $ti
    | .command = $cmd
  ' 2>/dev/null
)"
if [ -z "$UPDATED_INPUT" ]; then
  exit 0
fi

jq -n --argjson updated "$UPDATED_INPUT" '{
  hookSpecificOutput: {
    hookEventName: "PreToolUse",
    permissionDecision: "allow",
    permissionDecisionReason: "ccp auto-rewrite",
    updatedInput: $updated
  }
}'
`
}
