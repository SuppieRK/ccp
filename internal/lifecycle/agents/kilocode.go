package agents

import "path/filepath"

const kilocodeRulePath = ".kilocode/rules/ccp.md"

type KilocodeAdapter struct {
	ManagedRepoRuleFileAdapter
}

func NewKilocodeAdapter() KilocodeAdapter {
	return KilocodeAdapter{
		ManagedRepoRuleFileAdapter: NewManagedRepoRuleFileAdapter(
			string(AgentKilocode),
			".kilocode",
			kilocodeRulePath,
			"missing kilocode rule file: %s",
			"missing kilocode managed guidance in %s",
			kilocodeRuleContent,
			[]string{
				"## CCP Integration (Managed)",
				"Use `ccp` as the command prefix for every executable in shell commands",
				"`ccp nl -ba spec.md | ccp sed -n '1,260p'`",
				ccpRawEscapeHatch,
			},
		),
	}
}

func (a KilocodeAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".kilocode")
}

func kilocodeRuleContent() string {
	return ccpManagedGuidanceMarkdown()
}
