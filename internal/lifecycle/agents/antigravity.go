package agents

import "path/filepath"

const antigravityRulePath = ".agent/rules/ccp.md"

type AntigravityAdapter struct {
	ManagedRepoRuleFileAdapter
}

func NewAntigravityAdapter() AntigravityAdapter {
	return AntigravityAdapter{
		ManagedRepoRuleFileAdapter: NewManagedRepoRuleFileAdapter(
			string(AgentAntigravity),
			".agent",
			antigravityRulePath,
			"missing antigravity rule file: %s",
			"missing antigravity managed guidance in %s",
			antigravityRuleContent,
			[]string{
				"## CCP Integration (Managed)",
				"Use `ccp` as the command prefix for every executable in shell commands",
				"`ccp nl -ba spec.md | ccp sed -n '1,260p'`",
				ccpRawEscapeHatch,
			},
		),
	}
}

func (a AntigravityAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".agent")
}

func antigravityRuleContent() string {
	return ccpManagedGuidanceMarkdown()
}
