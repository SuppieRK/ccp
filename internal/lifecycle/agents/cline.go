package agents

import "path/filepath"

const clineRulePath = ".clinerules/ccp.md"

type ClineAdapter struct {
	ManagedRepoRuleFileAdapter
}

func NewClineAdapter() ClineAdapter {
	return ClineAdapter{
		ManagedRepoRuleFileAdapter: NewManagedRepoRuleFileAdapter(
			string(AgentCline),
			".clinerules",
			clineRulePath,
			"missing cline rule file: %s",
			"missing cline managed guidance in %s",
			clineRuleContent,
			[]string{
				"## CCP Integration (Managed)",
				"Use `ccp` as the command prefix for every executable in shell commands",
				"`ccp nl -ba spec.md | ccp sed -n '1,260p'`",
				ccpRawEscapeHatch,
			},
		),
	}
}

func (a ClineAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".clinerules")
}

func clineRuleContent() string {
	return ccpManagedGuidanceMarkdown()
}
