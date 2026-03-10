package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	qwenAgentsPath   = ".qwen/AGENTS.md"
	qwenSettingsPath = ".qwen/settings.json"
	qwenAgentsFile   = "AGENTS.md"
)

type QwenAdapter struct{}

func NewQwenAdapter() QwenAdapter {
	return QwenAdapter{}
}

func (a QwenAdapter) ID() string { return string(AgentQwen) }

func (a QwenAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".qwen")
}

func (a QwenAdapter) Plan(ctx Context) []PlannedArtifact {
	return []PlannedArtifact{
		{
			Kind:    ArtifactSettings,
			Path:    ResolveHomeScopedPath(ctx.HomeDir, qwenSettingsPath),
			Content: "{\n  \"context\": {\n    \"fileName\": \"" + qwenAgentsFile + "\"\n  }\n}\n",
			Perm:    0o644,
		},
		{
			Kind:    ArtifactSettings,
			Path:    ResolveHomeScopedPath(ctx.HomeDir, qwenAgentsPath),
			Content: ccpManagedBlockTemplate(),
			Perm:    0o644,
		},
	}
}

func (a QwenAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	settingsPath := ResolveHomeScopedPath(ctx.HomeDir, qwenSettingsPath)
	content, err := upsertQwenSettings(settingsPath)
	if err != nil {
		return InstallResult{}, err
	}
	settingsChanged, err := write(settingsPath, []byte(content), 0o644)
	if err != nil {
		return InstallResult{}, err
	}

	agentsPath := ResolveHomeScopedPath(ctx.HomeDir, qwenAgentsPath)
	agentsContent, err := upsertManagedInstructionBlock(agentsPath)
	if err != nil {
		return InstallResult{}, err
	}
	agentsChanged, err := write(agentsPath, []byte(agentsContent), 0o644)
	if err != nil {
		return InstallResult{}, err
	}

	res := InstallResult{}
	for _, changed := range []bool{settingsChanged, agentsChanged} {
		if changed {
			res.Applied++
		} else {
			res.Noop++
		}
	}
	return res, nil
}

func (a QwenAdapter) Verify(ctx Context) error {
	settingsPath := ResolveHomeScopedPath(ctx.HomeDir, qwenSettingsPath)
	ok, err := qwenSettingsUseAgents(settingsPath)
	if err != nil {
		return fmt.Errorf("missing qwen settings file: %s", settingsPath)
	}
	if !ok {
		return fmt.Errorf("missing qwen managed context filename in %s", settingsPath)
	}
	agentsPath := ResolveHomeScopedPath(ctx.HomeDir, qwenAgentsPath)
	return verifyManagedInstructionBlock(agentsPath, "missing qwen agents file: %s", "missing qwen managed block markers in %s")
}

func (a QwenAdapter) Uninstall(ctx Context) (InstallResult, error) {
	settingsPath := ResolveHomeScopedPath(ctx.HomeDir, qwenSettingsPath)
	updated, changed, removeAll, err := removeQwenSettings(settingsPath)
	if err != nil {
		return InstallResult{}, err
	}

	res, err := applyQwenUninstallChange(settingsPath, updated, changed, removeAll)
	if err != nil {
		return InstallResult{}, err
	}

	agentsPath := ResolveHomeScopedPath(ctx.HomeDir, qwenAgentsPath)
	updatedAgents, changedAgents, removeAllAgents, err := removeManagedInstructionBlock(agentsPath)
	if err != nil {
		return InstallResult{}, err
	}
	agentsRes, err := applyQwenUninstallChange(agentsPath, updatedAgents, changedAgents, removeAllAgents)
	if err != nil {
		return InstallResult{}, err
	}
	res.Applied += agentsRes.Applied
	res.Noop += agentsRes.Noop
	return res, nil
}

func applyQwenUninstallChange(path, updated string, changed, removeAll bool) (InstallResult, error) {
	if !changed {
		return InstallResult{Noop: 1}, nil
	}
	if removeAll {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return InstallResult{}, err
		}
		return InstallResult{Applied: 1}, nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{Applied: 1}, nil
}

func upsertQwenSettings(path string) (string, error) {
	root, err := readQwenSettings(path)
	if err != nil {
		return "", err
	}
	contextMap, _ := root["context"].(map[string]any)
	if contextMap == nil {
		contextMap = map[string]any{}
	}
	contextMap["fileName"] = qwenAgentsFile
	root["context"] = contextMap
	return marshalQwenSettings(root)
}

func removeQwenSettings(path string) (updated string, changed bool, removeAll bool, err error) {
	root, err := readQwenSettings(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, false, nil
		}
		return "", false, false, err
	}
	contextMap, _ := root["context"].(map[string]any)
	if contextMap == nil {
		return "", false, false, nil
	}
	if v, _ := contextMap["fileName"].(string); v != qwenAgentsFile {
		return "", false, false, nil
	}
	delete(contextMap, "fileName")
	if len(contextMap) == 0 {
		delete(root, "context")
	} else {
		root["context"] = contextMap
	}
	if len(root) == 0 {
		return "", true, true, nil
	}
	content, err := marshalQwenSettings(root)
	if err != nil {
		return "", false, false, err
	}
	return content, true, false, nil
}

func qwenSettingsUseAgents(path string) (bool, error) {
	root, err := readQwenSettings(path)
	if err != nil {
		return false, err
	}
	contextMap, _ := root["context"].(map[string]any)
	if contextMap == nil {
		return false, nil
	}
	return contextMap["fileName"] == qwenAgentsFile, nil
}

func readQwenSettings(path string) (map[string]any, error) {
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

func marshalQwenSettings(root map[string]any) (string, error) {
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	return string(append(out, '\n')), nil
}
