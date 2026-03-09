package agents

import "path/filepath"

const iflowMemoryPath = "IFLOW.md"

type IFlowAdapter struct {
	ManagedRepoInstructionFileAdapter
}

func NewIFlowAdapter() IFlowAdapter {
	return IFlowAdapter{
		ManagedRepoInstructionFileAdapter: NewManagedRepoInstructionFileAdapter(
			string(AgentIFlow),
			".iflow",
			iflowMemoryPath,
			"missing iflow memory file: %s",
			"missing iflow managed block markers in %s",
		),
	}
}

func (a IFlowAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".iflow")
}
