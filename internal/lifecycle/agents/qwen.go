package agents

import "path/filepath"

const qwenAgentsPath = "AGENTS.md"

type QwenAdapter struct {
	ManagedRepoInstructionFileAdapter
}

func NewQwenAdapter() QwenAdapter {
	return QwenAdapter{
		ManagedRepoInstructionFileAdapter: NewManagedRepoInstructionFileAdapter(
			string(AgentQwen),
			".qwen",
			qwenAgentsPath,
			"missing qwen agents file: %s",
			"missing qwen managed block markers in %s",
		),
	}
}

func (a QwenAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".qwen")
}
