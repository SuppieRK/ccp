package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	windsurfHookScriptName = "ccp-block.sh"
	windsurfHooksName      = "hooks.json"
)

type WindsurfAdapter struct{}

func NewWindsurfAdapter() WindsurfAdapter {
	return WindsurfAdapter{}
}

func (a WindsurfAdapter) ID() string { return string(AgentWindsurf) }

func (a WindsurfAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".windsurf")
}

func (a WindsurfAdapter) Plan(ctx Context) []PlannedArtifact {
	root := windsurfRoot(ctx)
	hookPath := filepath.Join(root, "hooks", windsurfHookScriptName)
	hooksPath := filepath.Join(root, windsurfHooksName)
	return []PlannedArtifact{
		{
			Kind:    ArtifactHook,
			Path:    hookPath,
			Content: windsurfHookScriptContent(),
			Perm:    0o755,
		},
		{
			Kind:    ArtifactSettings,
			Path:    hooksPath,
			Content: fmt.Sprintf("{\n  \"pre_run_command\": [\n    {\n      \"command\": \"%s\",\n      \"enabled\": true,\n      \"name\": \"ccp-pre-run-command\"\n    }\n  ]\n}\n", strings.ReplaceAll(hookPath, "\\", "\\\\")),
			Perm:    0o644,
		},
	}
}

func (a WindsurfAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	root := windsurfRoot(ctx)
	hookPath := filepath.Join(root, "hooks", windsurfHookScriptName)
	hooksPath := filepath.Join(root, windsurfHooksName)

	res := InstallResult{}
	scriptChanged, err := write(hookPath, []byte(windsurfHookScriptContent()), 0o755)
	if err != nil {
		return InstallResult{}, err
	}
	if scriptChanged {
		res.Applied++
	} else {
		res.Noop++
	}

	content, err := upsertWindsurfHooksConfig(hooksPath, hookPath)
	if err != nil {
		return InstallResult{}, err
	}
	settingsChanged, err := write(hooksPath, []byte(content), 0o644)
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

func (a WindsurfAdapter) Verify(ctx Context) error {
	root := windsurfRoot(ctx)
	hookPath := filepath.Join(root, "hooks", windsurfHookScriptName)
	hooksPath := filepath.Join(root, windsurfHooksName)

	for _, check := range []struct {
		path string
		msg  string
	}{
		{path: hookPath, msg: "missing windsurf hook script: %s"},
		{path: hooksPath, msg: "missing windsurf hooks file: %s"},
	} {
		if _, err := os.Stat(check.path); err != nil {
			return fmt.Errorf(check.msg, check.path)
		}
	}

	ok, err := windsurfHooksConfigHasEntry(hooksPath, hookPath)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("missing windsurf managed hook contribution in %s", hooksPath)
	}
	return nil
}

func (a WindsurfAdapter) Uninstall(ctx Context) (InstallResult, error) {
	root := windsurfRoot(ctx)
	hookPath := filepath.Join(root, "hooks", windsurfHookScriptName)
	hooksPath := filepath.Join(root, windsurfHooksName)

	res := InstallResult{}
	removed, err := removeFileIfExists(hookPath)
	if err != nil {
		return InstallResult{}, err
	}
	if removed {
		res.Applied++
	}

	updated, changed, removeAll, err := removeWindsurfHooksConfig(hooksPath, hookPath)
	if err != nil {
		return InstallResult{}, err
	}
	if !changed {
		res.Noop++
		return res, nil
	}
	if removeAll {
		if err := os.Remove(hooksPath); err != nil && !os.IsNotExist(err) {
			return InstallResult{}, err
		}
		res.Applied++
		return res, nil
	}
	if err := os.WriteFile(hooksPath, []byte(updated), 0o644); err != nil {
		return InstallResult{}, err
	}
	res.Applied++
	return res, nil
}

func windsurfRoot(ctx Context) string {
	base := ctx.HomeDir
	if strings.TrimSpace(base) == "" {
		base = ctx.ScopeRoot
	}
	return filepath.Join(base, ".codeium", "windsurf")
}

func windsurfHookScriptContent() string {
	return bashBlockingHookScriptContent(
		"windsurf",
		"Windsurf pre_run_command hook: block shell commands that do not use the ccp prefix.",
		"ccp-windsurf-hook.log",
		"",
		"command",
		`printf 'Use ccp as the command prefix for shell commands. Retry as: ccp %s\n' "$CMD" >&2`,
	)
}

func upsertWindsurfHooksConfig(path, hookPath string) (string, error) {
	root, err := readWindsurfHooksConfig(path)
	if err != nil {
		return "", err
	}
	key := "pre_run_command"
	entries := normalizeWindsurfHookEntries(root[key])
	if !windsurfHookEntriesContain(entries, hookPath) {
		entries = append(entries, map[string]any{
			"name":    "ccp-pre-run-command",
			"command": hookPath,
			"enabled": true,
		})
	}
	root[key] = entries
	return marshalWindsurfHooksConfig(root)
}

func removeWindsurfHooksConfig(path, hookPath string) (updated string, changed bool, removeAll bool, err error) {
	root, err := readWindsurfHooksConfig(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, false, nil
		}
		return "", false, false, err
	}

	key := "pre_run_command"
	entries := normalizeWindsurfHookEntries(root[key])
	next := make([]map[string]any, 0, len(entries))
	found := false
	for _, entry := range entries {
		cmd, _ := entry["command"].(string)
		if filepath.Clean(strings.TrimSpace(cmd)) == filepath.Clean(strings.TrimSpace(hookPath)) {
			found = true
			continue
		}
		next = append(next, entry)
	}
	if !found {
		return "", false, false, nil
	}
	if len(next) == 0 {
		delete(root, key)
	} else {
		root[key] = next
	}
	if len(root) == 0 {
		return "", true, true, nil
	}
	content, err := marshalWindsurfHooksConfig(root)
	if err != nil {
		return "", false, false, err
	}
	return content, true, false, nil
}

func windsurfHooksConfigHasEntry(path, hookPath string) (bool, error) {
	root, err := readWindsurfHooksConfig(path)
	if err != nil {
		return false, err
	}
	return windsurfHookEntriesContain(normalizeWindsurfHookEntries(root["pre_run_command"]), hookPath), nil
}

func readWindsurfHooksConfig(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	root := map[string]any{}
	if len(raw) == 0 {
		return root, nil
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	return root, nil
}

func marshalWindsurfHooksConfig(root map[string]any) (string, error) {
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	return string(append(out, '\n')), nil
}

func normalizeWindsurfHookEntries(v any) []map[string]any {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if ok {
			out = append(out, entry)
		}
	}
	return out
}

func windsurfHookEntriesContain(entries []map[string]any, hookPath string) bool {
	for _, entry := range entries {
		cmd, _ := entry["command"].(string)
		if filepath.Clean(strings.TrimSpace(cmd)) == filepath.Clean(strings.TrimSpace(hookPath)) {
			return true
		}
	}
	return false
}
