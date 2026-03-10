package agents

import "path/filepath"

const factoryAgentsPath = ".factory/AGENTS.md"

type FactoryAdapter struct {
	ManagedInstructionFileAdapter
}

func NewFactoryAdapter() FactoryAdapter {
	return FactoryAdapter{
		ManagedInstructionFileAdapter: NewManagedInstructionFileAdapter(
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

func (a FactoryAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	return a.ManagedInstructionFileAdapter.Install(ctx, write)
}
