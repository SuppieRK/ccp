package agents

import "path/filepath"

const githubCopilotInstructionsPath = ".copilot/copilot-instructions.md"

type GitHubCopilotAdapter struct {
	ManagedInstructionFileAdapter
}

func NewGitHubCopilotAdapter() GitHubCopilotAdapter {
	return GitHubCopilotAdapter{
		ManagedInstructionFileAdapter: NewManagedInstructionFileAdapter(
			string(AgentGitHubCopilot),
			".github",
			githubCopilotInstructionsPath,
			"missing github copilot instructions file: %s",
			"missing github copilot managed block markers in %s",
		),
	}
}

func (a GitHubCopilotAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".github")
}
