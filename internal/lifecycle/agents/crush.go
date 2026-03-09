package agents

import "path/filepath"

const crushAgentsPath = "AGENTS.md"

type CrushAdapter struct {
	ManagedRepoInstructionFileAdapter
}

func NewCrushAdapter() CrushAdapter {
	return CrushAdapter{
		ManagedRepoInstructionFileAdapter: NewManagedRepoInstructionFileAdapter(
			string(AgentCrush),
			".crush",
			crushAgentsPath,
			"missing crush agents file: %s",
			"missing crush managed block markers in %s",
		),
	}
}

func (a CrushAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".crush")
}
