package gitfilters

import "go-command-compression-proxy/internal/engine"

// NewGitMergeFilter compacts merge success and preserves failure diagnostics.
func NewGitMergeFilter() engine.ToolFilter { return gitMergeFilter{} }

type gitMergeFilter struct{}

func (gitMergeFilter) Tool() string      { return "git merge" }
func (gitMergeFilter) Aliases() []string { return nil }
func (gitMergeFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}
func (gitMergeFilter) ContextKey(ev engine.Event) string { return sharedContextKey(ev) }
func (gitMergeFilter) MaskingHorizon() int               { return 0 }
func (gitMergeFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	return genericWriteProcess(ev, mem, "git merge: ok")
}
