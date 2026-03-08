package agents

import "path/filepath"

const qoderAgentsPath = "AGENTS.md"

type QoderAdapter struct {
	ManagedRepoInstructionFileAdapter
}

func NewQoderAdapter() QoderAdapter {
	return QoderAdapter{
		ManagedRepoInstructionFileAdapter: NewManagedRepoInstructionFileAdapter(
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
