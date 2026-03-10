package agents

import "path/filepath"

const qoderAgentsPath = ".qoder/AGENTS.md"

type QoderAdapter struct {
	ManagedInstructionFileAdapter
}

func NewQoderAdapter() QoderAdapter {
	return QoderAdapter{
		ManagedInstructionFileAdapter: NewManagedInstructionFileAdapter(
			string(AgentQoder),
			".qoder",
			qoderAgentsPath,
			"missing qoder agents file: %s",
			"missing qoder managed block markers in %s",
		),
	}
}

func (a QoderAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".qoder")
}
