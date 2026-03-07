package agents

import "path/filepath"

const kiroSteeringPath = ".kiro/steering/ccp.md"

type KiroAdapter struct {
	ManagedRepoRuleFileAdapter
}

func NewKiroAdapter() KiroAdapter {
	return KiroAdapter{
		ManagedRepoRuleFileAdapter: NewManagedRepoRuleFileAdapter(
			string(AgentKiro),
			".kiro",
			kiroSteeringPath,
			"missing kiro steering file: %s",
			"missing kiro managed guidance in %s",
			kiroSteeringContent,
			[]string{
				"## CCP Integration (Managed)",
				"Use `ccp` as the command prefix for every executable in shell commands",
				"`ccp nl -ba spec.md | ccp sed -n '1,260p'`",
			},
		),
	}
}

func (a KiroAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".kiro")
}

func kiroSteeringContent() string {
	return ccpManagedGuidanceMarkdown()
}
