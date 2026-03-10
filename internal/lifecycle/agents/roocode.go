package agents

import "path/filepath"

const roocodeRulePath = ".roo/rules/ccp.md"

type RooCodeAdapter struct {
	ManagedHomeRuleFileAdapter
}

func NewRooCodeAdapter() RooCodeAdapter {
	return RooCodeAdapter{
		ManagedHomeRuleFileAdapter: NewManagedHomeRuleFileAdapter(
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
				ccpRawEscapeHatch,
			},
		),
	}
}

func (a RooCodeAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".roo")
}

func (a RooCodeAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	return a.ManagedHomeRuleFileAdapter.Install(ctx, write)
}

func roocodeRuleContent() string {
	return ccpManagedGuidanceMarkdown()
}
