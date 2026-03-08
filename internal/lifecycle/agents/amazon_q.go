package agents

import "path/filepath"

const amazonQRulePath = ".amazonq/rules/ccp.md"

type AmazonQAdapter struct {
	ManagedRepoRuleFileAdapter
}

func NewAmazonQAdapter() AmazonQAdapter {
	return AmazonQAdapter{
		ManagedRepoRuleFileAdapter: NewManagedRepoRuleFileAdapter(
			string(AgentAmazonQ),
			".amazonq",
			amazonQRulePath,
			"missing amazon q rule file: %s",
			"missing amazon q managed guidance in %s",
			amazonQRuleContent,
			[]string{
				"## CCP Integration (Managed)",
				"Use `ccp` as the command prefix for every executable in shell commands",
				"`ccp nl -ba spec.md | ccp sed -n '1,260p'`",
				ccpRawEscapeHatch,
			},
		),
	}
}

func (a AmazonQAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".amazonq")
}

func amazonQRuleContent() string {
	return ccpManagedGuidanceMarkdown()
}
