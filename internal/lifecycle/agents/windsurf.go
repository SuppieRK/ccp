package agents

import "path/filepath"

const windsurfRulePath = ".windsurf/rules/ccp.md"

type WindsurfAdapter struct {
	ManagedRepoRuleFileAdapter
}

func NewWindsurfAdapter() WindsurfAdapter {
	return WindsurfAdapter{
		ManagedRepoRuleFileAdapter: NewManagedRepoRuleFileAdapter(
			string(AgentWindsurf),
			".windsurf",
			windsurfRulePath,
			"missing windsurf rule file: %s",
			"missing windsurf managed guidance in %s",
			windsurfRuleContent,
			[]string{
				"trigger: always_on",
				"## CCP Integration (Managed)",
				"Use `ccp` as the command prefix for every executable in shell commands",
				"`ccp nl -ba spec.md | ccp sed -n '1,260p'`",
				ccpRawEscapeHatch,
			},
		),
	}
}

func (a WindsurfAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".windsurf")
}

func windsurfRuleContent() string {
	return "---\n" +
		"trigger: always_on\n" +
		"---\n\n" +
		ccpManagedGuidanceMarkdown()
}
