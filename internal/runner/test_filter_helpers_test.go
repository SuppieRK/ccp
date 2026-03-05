package runner

import "go-command-compression-proxy/internal/engine"

type runnerTestFilterBase struct {
	tool          string
	aliases       []string
	sharedContext bool
}

func (b runnerTestFilterBase) Tool() string      { return b.tool }
func (b runnerTestFilterBase) Aliases() []string { return b.aliases }
func (b runnerTestFilterBase) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}
func (b runnerTestFilterBase) ContextKey(ev engine.Event) string {
	if b.sharedContext {
		return engine.SharedContextKey(ev)
	}
	return ev.CommandID + ":" + ev.Tool + ":" + string(ev.Stream)
}
func (b runnerTestFilterBase) MaskingHorizon() int { return 0 }
