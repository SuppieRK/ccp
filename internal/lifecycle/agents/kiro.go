package agents

import "path/filepath"

const kiroSteeringPath = ".kiro/steering/AGENTS.md"

type KiroAdapter struct {
	ManagedInstructionFileAdapter
}

func NewKiroAdapter() KiroAdapter {
	return KiroAdapter{
		ManagedInstructionFileAdapter: NewManagedInstructionFileAdapter(
			string(AgentKiro),
			".kiro",
			kiroSteeringPath,
			"missing kiro steering file: %s",
			"missing kiro managed block markers in %s",
		),
	}
}

func (a KiroAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".kiro")
}

func (a KiroAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	return a.ManagedInstructionFileAdapter.Install(ctx, write)
}
