package agents

import "path/filepath"

const piAgentsPath = "AGENTS.md"

type PiAdapter struct {
	ManagedRepoInstructionFileAdapter
}

func NewPiAdapter() PiAdapter {
	return PiAdapter{
		ManagedRepoInstructionFileAdapter: NewManagedRepoInstructionFileAdapter(
			string(AgentPi),
			".pi",
			piAgentsPath,
			"missing pi agents file: %s",
			"missing pi managed block markers in %s",
		),
	}
}

func (a PiAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".pi")
}
