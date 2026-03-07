package agents

import "path/filepath"

const geminiInstructionsPath = ".gemini/GEMINI.md"

type GeminiAdapter struct {
	ManagedInstructionFileAdapter
}

func NewGeminiAdapter() GeminiAdapter {
	return GeminiAdapter{
		ManagedInstructionFileAdapter: NewManagedInstructionFileAdapter(
			string(AgentGemini),
			".gemini",
			geminiInstructionsPath,
			"missing gemini instructions file: %s",
			"missing gemini managed block markers in %s",
		),
	}
}

func (a GeminiAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".gemini")
}
