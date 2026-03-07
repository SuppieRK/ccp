package agents

import "path/filepath"

const roocodeRulePath = ".roo/rules/ccp.md"

type RooCodeAdapter struct {
	ManagedRepoRuleFileAdapter
}

func NewRooCodeAdapter() RooCodeAdapter {
	return RooCodeAdapter{
		ManagedRepoRuleFileAdapter: NewManagedRepoRuleFileAdapter(
			string(AgentRooCode),
			".roo",
			roocodeRulePath,
			"missing roocode rule file: %s",
			"missing roocode managed guidance in %s",
			roocodeRuleContent,
			[]string{
				"## CCP Integration (Managed)",
				"Use `ccp` as the command prefix for every executable in shell commands",
				"`ccp nl -ba spec.md | ccp sed -n '1,260p'`",
			},
		),
	}
}

func (a RooCodeAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".roo")
}

func roocodeRuleContent() string {
	return ccpManagedGuidanceMarkdown()
}
