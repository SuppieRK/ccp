package agents

import "path/filepath"

const factoryAgentsPath = "AGENTS.md"

type FactoryAdapter struct {
	ManagedRepoInstructionFileAdapter
}

func NewFactoryAdapter() FactoryAdapter {
	return FactoryAdapter{
		ManagedRepoInstructionFileAdapter: NewManagedRepoInstructionFileAdapter(
			string(AgentFactory),
			".factory",
			factoryAgentsPath,
			"missing factory agents file: %s",
			"missing factory managed block markers in %s",
		),
	}
}

func (a FactoryAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".factory")
}
