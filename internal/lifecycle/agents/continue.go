package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	continueHookScriptName = "ccp-rewrite.sh"
	continueSettingsName   = "settings.json"
)

type ContinueAdapter struct{}

func NewContinueAdapter() ContinueAdapter {
	return ContinueAdapter{}
}

func (a ContinueAdapter) ID() string { return string(AgentContinue) }

func (a ContinueAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".continue")
}

func (a ContinueAdapter) Plan(ctx Context) []PlannedArtifact {
	root := continueRoot(ctx)
	hookPath := filepath.Join(root, "hooks", continueHookScriptName)
	settingsPath := filepath.Join(root, continueSettingsName)
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

func (a ContinueAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	return InstallPlannedArtifacts(a.Plan(ctx), write)
}

func (a ContinueAdapter) Verify(ctx Context) error {
	root := continueRoot(ctx)
	for _, check := range []struct {
		path string
		msg  string
	}{
		{path: filepath.Join(root, "hooks", continueHookScriptName), msg: "missing continue hook script: %s"},
		{path: filepath.Join(root, continueSettingsName), msg: "missing continue settings file: %s"},
	} {
		if _, err := os.Stat(check.path); err != nil {
			return fmt.Errorf(check.msg, check.path)
		}
	}
	return nil
}

func (a ContinueAdapter) Uninstall(ctx Context) (InstallResult, error) {
	root := continueRoot(ctx)
	hookPath := filepath.Join(root, "hooks", continueHookScriptName)
	settingsPath := filepath.Join(root, continueSettingsName)

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

func continueRoot(ctx Context) string {
	base := ctx.HomeDir
	if strings.TrimSpace(base) == "" {
		base = ctx.ScopeRoot
	}
	return filepath.Join(base, ".continue")
}
