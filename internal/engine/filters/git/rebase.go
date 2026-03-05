package gitfilters

import "go-command-compression-proxy/internal/engine"

// NewGitRebaseFilter compacts rebase success and preserves failure diagnostics.
func NewGitRebaseFilter() engine.ToolFilter { return gitRebaseFilter{} }

type gitRebaseFilter struct{}

func (gitRebaseFilter) Tool() string      { return "git rebase" }
func (gitRebaseFilter) Aliases() []string { return nil }
func (gitRebaseFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}
func (gitRebaseFilter) ContextKey(ev engine.Event) string { return sharedContextKey(ev) }
func (gitRebaseFilter) MaskingHorizon() int               { return 0 }
func (gitRebaseFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	return genericWriteProcess(ev, mem, "ok rebase")
}
