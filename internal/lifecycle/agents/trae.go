package agents

import "path/filepath"

const traeRulePath = ".trae/rules/ccp.md"

type TraeAdapter struct {
	ManagedRepoRuleFileAdapter
}

func NewTraeAdapter() TraeAdapter {
	return TraeAdapter{
		ManagedRepoRuleFileAdapter: NewManagedRepoRuleFileAdapter(
			string(AgentTrae),
			".trae",
			traeRulePath,
			"missing trae rule file: %s",
			"missing trae managed guidance in %s",
			traeRuleContent,
			[]string{
				"## CCP Integration (Managed)",
				"Use `ccp` as the command prefix for every executable in shell commands",
				"`ccp nl -ba spec.md | ccp sed -n '1,260p'`",
			},
		),
	}
}

func (a TraeAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".trae")
}

func traeRuleContent() string {
	return ccpManagedGuidanceMarkdown()
}
