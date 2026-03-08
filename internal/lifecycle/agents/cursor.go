package agents

import "path/filepath"

const cursorRulePath = ".cursor/rules/ccp.mdc"

type CursorAdapter struct {
	ManagedRepoRuleFileAdapter
}

func NewCursorAdapter() CursorAdapter {
	return CursorAdapter{
		ManagedRepoRuleFileAdapter: NewManagedRepoRuleFileAdapter(
			string(AgentCursor),
			".cursor",
			cursorRulePath,
			"missing cursor rule file: %s",
			"missing cursor managed guidance in %s",
			cursorRuleContent,
			[]string{
				"alwaysApply: true",
				"Use `ccp` as the command prefix for every executable in shell commands",
				"`ccp nl -ba spec.md | ccp sed -n '1,260p'`",
				ccpRawEscapeHatch,
			},
		),
	}
}

func (a CursorAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".cursor")
}

func cursorRuleContent() string {
	return "---\n" +
		"description: Route shell commands through ccp\n" +
		"alwaysApply: true\n" +
		"---\n\n" +
		ccpManagedGuidanceMarkdown()
}
