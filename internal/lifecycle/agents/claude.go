package agents

import (
	"fmt"
	"go-command-compression-proxy/internal/projectfiles"
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

func (a ClaudeAdapter) ID() string { return "claude" }

func (a ClaudeAdapter) DetectRoot(scopeRoot string) string {

	return filepath.Join(scopeRoot, ".claude")
}

func (a ClaudeAdapter) Plan(ctx Context) []PlannedArtifact {
	root := claudeRoot(ctx)
	awarenessPath := filepath.Join(root, claudeAwarenessName)
	guidePath := filepath.Join(root, claudeGuideName)
	plan := bashHookAndSettingsArtifacts(root, claudeHookScriptName, claudeSettingsName, claudeHookScriptContent())
	plan = append(plan,
		PlannedArtifact{
			Kind:    ArtifactAwareness,
			Path:    awarenessPath,
			Content: awarenessContent(a.ID()),
			Perm:    0o644,
		},
		PlannedArtifact{
			Kind:    ArtifactAwareness,
			Path:    guidePath,
			Content: claudeManagedGuideBlock(),
			Perm:    0o644,
		},
	)
	return plan
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
	if err := verifyArtifactFiles(
		artifactCheck{path: filepath.Join(root, "hooks", claudeHookScriptName), msg: "missing hook script: %s"},
		artifactCheck{path: filepath.Join(root, claudeSettingsName), msg: "missing settings file: %s"},
		artifactCheck{path: filepath.Join(root, claudeAwarenessName), msg: "missing awareness file: %s"},
		artifactCheck{path: filepath.Join(root, claudeGuideName), msg: "missing claude guide file: %s"},
	); err != nil {
		return err
	}
	if err := verifyClaudeGuideBlock(filepath.Join(root, claudeGuideName)); err != nil {
		return err
	}
	return nil
}

func claudeRoot(ctx Context) string {
	return homeOrScopePath(ctx, ".claude")
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
	changed, err := removePreToolUseCommandHook(settingsPath, hookPath)
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
	guideRes, err := applyManagedFileChange(guidePath, updatedGuide, changedGuide, removeAllGuide)
	if err != nil {
		return err
	}
	res.Applied += guideRes.Applied
	res.Noop += guideRes.Noop
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
	return removeManagedContextBlock(path)
}

func removeFileIfExists(path string) (bool, error) {
	if err := projectfiles.RejectSymlinkPath(path); err != nil {
		return false, err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func claudeHookScriptContent() string {
	return bashRewriteHookScriptContent("claude", "ccp-claude-hook.log")
}

func awarenessContent(toolID string) string {
	return fmt.Sprintf("# CCP Proxy Integration\n\nTool: %s\n\nCommands are routed through `ccp` via hook wiring installed by `ccp init`.\n", toolID)
}
