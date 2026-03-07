package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const cursorRulePath = ".cursor/rules/ccp.mdc"

type CursorAdapter struct{}

func NewCursorAdapter() CursorAdapter { return CursorAdapter{} }

func (a CursorAdapter) ID() string { return string(AgentCursor) }

func (a CursorAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".cursor")
}

func (a CursorAdapter) Plan(ctx Context) []PlannedArtifact {
	return []PlannedArtifact{{
		Kind:    ArtifactSettings,
		Path:    a.targetPath(ctx),
		Content: cursorRuleContent(),
		Perm:    0o644,
	}}
}

func (a CursorAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	changed, err := write(a.targetPath(ctx), []byte(cursorRuleContent()), 0o644)
	if err != nil {
		return InstallResult{}, err
	}
	if !changed {
		return InstallResult{Noop: 1}, nil
	}
	return InstallResult{Applied: 1}, nil
}

func (a CursorAdapter) Verify(ctx Context) error {
	target := a.targetPath(ctx)
	data, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("missing cursor rule file: %s", target)
	}
	content := string(data)
	for _, needle := range []string{
		"alwaysApply: true",
		"Use `ccp` as the command prefix for every executable in shell commands",
		"`ccp nl -ba spec.md | ccp sed -n '1,260p'`",
	} {
		if !strings.Contains(content, needle) {
			return fmt.Errorf("missing cursor managed guidance in %s", target)
		}
	}
	return nil
}

func (a CursorAdapter) Uninstall(ctx Context) (InstallResult, error) {
	target := a.targetPath(ctx)
	if err := os.Remove(target); err != nil {
		if os.IsNotExist(err) {
			return InstallResult{Noop: 1}, nil
		}
		return InstallResult{}, err
	}
	return InstallResult{Applied: 1}, nil
}

func (a CursorAdapter) targetPath(ctx Context) string {
	return filepath.Join(ctx.ScopeRoot, cursorRulePath)
}

func cursorRuleContent() string {
	return "---\n" +
		"description: Route shell commands through ccp\n" +
		"alwaysApply: true\n" +
		"---\n\n" +
		ccpManagedGuidanceMarkdown()
}
