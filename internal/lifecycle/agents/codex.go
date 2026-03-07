package agents

import (
	"path/filepath"
)

const (
	codexAgentsPath = ".codex/AGENTS.md"
)

type CodexAdapter struct {
	ManagedInstructionFileAdapter
}

func NewCodexAdapter() CodexAdapter {
	return CodexAdapter{
		ManagedInstructionFileAdapter: NewManagedInstructionFileAdapter(
			string(AgentCodex),
			".codex",
			codexAgentsPath,
			"missing codex agents file: %s",
			"missing codex managed block markers in %s",
		),
	}
}

func (a CodexAdapter) DetectRoot(scopeRoot string) string {
	// Detection remains repo-root based while installation is global.
	return filepath.Join(scopeRoot, ".codex")
}
