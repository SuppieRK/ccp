package agents

import "path/filepath"

const auggieAgentsPath = "AGENTS.md"

type AuggieAdapter struct {
	ManagedRepoInstructionFileAdapter
}

func NewAuggieAdapter() AuggieAdapter {
	return AuggieAdapter{
		ManagedRepoInstructionFileAdapter: NewManagedRepoInstructionFileAdapter(
			string(AgentAuggie),
			".augment",
			auggieAgentsPath,
			"missing auggie agents file: %s",
			"missing auggie managed block markers in %s",
		),
	}
}

func (a AuggieAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".augment")
}
