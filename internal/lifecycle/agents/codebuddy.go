package agents

import "path/filepath"

const codebuddyMemoryPath = "CODEBUDDY.md"

type CodeBuddyAdapter struct {
	ManagedRepoInstructionFileAdapter
}

func NewCodeBuddyAdapter() CodeBuddyAdapter {
	return CodeBuddyAdapter{
		ManagedRepoInstructionFileAdapter: NewManagedRepoInstructionFileAdapter(
			string(AgentCodeBuddy),
			".codebuddy",
			codebuddyMemoryPath,
			"missing codebuddy memory file: %s",
			"missing codebuddy managed block markers in %s",
		),
	}
}

func (a CodeBuddyAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".codebuddy")
}
