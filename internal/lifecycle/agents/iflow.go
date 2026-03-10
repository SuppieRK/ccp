package agents

import "path/filepath"

const iflowMemoryPath = ".iflow/IFLOW.md"

type IFlowAdapter struct {
	ManagedInstructionFileAdapter
}

func NewIFlowAdapter() IFlowAdapter {
	return IFlowAdapter{
		ManagedInstructionFileAdapter: NewManagedInstructionFileAdapter(
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

func (a IFlowAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	return a.ManagedInstructionFileAdapter.Install(ctx, write)
}
