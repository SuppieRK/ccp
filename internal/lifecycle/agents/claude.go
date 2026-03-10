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
	claudeGuideName      = "CLAUDE.md"
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
	guidePath := filepath.Join(root, claudeGuideName)
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
		{
			Kind:    ArtifactAwareness,
			Path:    guidePath,
			Content: claudeManagedGuideBlock(),
			Perm:    0o644,
		},
	}
}

func (a ClaudeAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	plan := a.Plan(ctx)
	var res InstallResult
	for _, item := range plan {
		changed, err := installClaudeArtifact(item, write)
		if err != nil {
			return res, err
		}
		updateInstallResult(&res, changed)
	}
	return res, nil
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
		{path: filepath.Join(root, claudeGuideName), msg: "missing claude guide file: %s"},
	} {
		if _, err := os.Stat(check.path); err != nil {
			return fmt.Errorf(check.msg, check.path)
		}
	}
	if err := verifyClaudeGuideBlock(filepath.Join(root, claudeGuideName)); err != nil {
		return err
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
	guidePath := filepath.Join(root, claudeGuideName)

	var res InstallResult
	if err := removeClaudeArtifacts(&res, hookPath, awarenessPath); err != nil {
		return res, err
	}
	if err := uninstallClaudeSettings(&res, settingsPath, hookPath); err != nil {
		return res, err
	}
	if err := uninstallClaudeGuide(&res, guidePath); err != nil {
		return res, err
	}
	return res, nil
}

func installClaudeArtifact(item PlannedArtifact, write WriterFunc) (bool, error) {
	data, err := claudeArtifactContent(item)
	if err != nil {
		return false, err
	}
	changed, err := write(item.Path, []byte(data), item.Perm)
	if err != nil {
		return false, err
	}
	if item.Kind == ArtifactHook {
		if err := os.Chmod(item.Path, 0o755); err != nil {
			return false, err
		}
	}
	return changed, nil
}

func claudeArtifactContent(item PlannedArtifact) (string, error) {
	if filepath.Base(item.Path) != claudeGuideName {
		return item.Content, nil
	}
	return upsertClaudeGuideBlock(item.Path)
}

func updateInstallResult(res *InstallResult, changed bool) {
	if changed {
		res.Applied++
		return
	}
	res.Noop++
}

func removeClaudeArtifacts(res *InstallResult, paths ...string) error {
	for _, p := range paths {
		removed, err := removeFileIfExists(p)
		if err != nil {
			return err
		}
		if removed {
			res.Applied++
		}
	}
	return nil
}

func uninstallClaudeSettings(res *InstallResult, settingsPath, hookPath string) error {
	changed, err := removeClaudePreToolUseHook(settingsPath, hookPath)
	if err != nil {
		return err
	}
	updateInstallResult(res, changed)
	return nil
}

func uninstallClaudeGuide(res *InstallResult, guidePath string) error {
	updatedGuide, changedGuide, removeAllGuide, err := removeClaudeGuideBlock(guidePath)
	if err != nil {
		return err
	}
	if !changedGuide {
		res.Noop++
		return nil
	}
	if removeAllGuide {
		if err := os.Remove(guidePath); err != nil && !os.IsNotExist(err) {
			return err
		}
	} else if err := os.WriteFile(guidePath, []byte(updatedGuide), 0o644); err != nil {
		return err
	}
	res.Applied++
	return nil
}

func claudeManagedGuideBlock() string {
	return ccpManagedBlockStart + "\n" +
		"## CCP Integration (Managed)\n\n" +
		"@CCP.md\n" +
		ccpManagedBlockEnd + "\n"
}

func verifyClaudeGuideBlock(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("missing claude guide file: %s", path)
	}
	content := string(data)
	if !strings.Contains(content, ccpManagedBlockStart) || !strings.Contains(content, ccpManagedBlockEnd) {
		return fmt.Errorf("missing claude managed guide block markers in %s", path)
	}
	if !strings.Contains(content, "@CCP.md") {
		return fmt.Errorf("missing claude CCP guide reference in %s", path)
	}
	return nil
}

func upsertClaudeGuideBlock(path string) (string, error) {
	block := claudeManagedGuideBlock()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return block, nil
		}
		return "", err
	}
	existing := string(raw)
	start := strings.Index(existing, ccpManagedBlockStart)
	end := strings.Index(existing, ccpManagedBlockEnd)
	if start >= 0 && end >= start {
		end += len(ccpManagedBlockEnd)
		tailStart := skipSingleLF(existing, end)
		updated := existing[:start] + strings.TrimRight(block, "\n") + "\n" + existing[tailStart:]
		return normalizeManagedFile(updated), nil
	}
	trimmed := strings.TrimRight(existing, "\n")
	if trimmed == "" {
		return block, nil
	}
	return normalizeManagedFile(trimmed + "\n\n" + block), nil
}

func removeClaudeGuideBlock(path string) (updated string, changed bool, removeAll bool, err error) {
	return removeManagedInstructionBlock(path)
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
	return bashRewriteHookScriptContent("claude", "ccp-claude-hook.log")
}
