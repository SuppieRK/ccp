package agents

import "path/filepath"

type AntigravityAdapter struct {
	ManagedInstructionFileAdapter
}

func NewAntigravityAdapter() AntigravityAdapter {
	return AntigravityAdapter{
		ManagedInstructionFileAdapter: NewManagedInstructionFileAdapter(
			string(AgentAntigravity),
			".agent",
			geminiInstructionsPath,
			"missing antigravity gemini-family instructions file: %s",
			"missing antigravity managed block markers in %s",
		),
	}
}

func (a AntigravityAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".agent")
}

func (a AntigravityAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	return a.ManagedInstructionFileAdapter.Install(ctx, write)
}
