package agents

import "path/filepath"

const continueRulePath = ".continue/rules/ccp.md"

type ContinueAdapter struct {
	ManagedRepoRuleFileAdapter
}

func NewContinueAdapter() ContinueAdapter {
	return ContinueAdapter{
		ManagedRepoRuleFileAdapter: NewManagedRepoRuleFileAdapter(
			string(AgentContinue),
			".continue",
			continueRulePath,
			"missing continue rule file: %s",
			"missing continue managed guidance in %s",
			continueRuleContent,
			[]string{
				"## CCP Integration (Managed)",
				"Use `ccp` as the command prefix for every executable in shell commands",
				"`ccp nl -ba spec.md | ccp sed -n '1,260p'`",
				ccpRawEscapeHatch,
			},
		),
	}
}

func (a ContinueAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".continue")
}

func continueRuleContent() string {
	return ccpManagedGuidanceMarkdown()
}
