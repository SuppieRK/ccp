package cargofilters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
)

func NewBuildFilter() engine.ToolFilter { return buildFilter{} }

type buildFilter struct{}

func (buildFilter) Tool() string      { return "cargo build" }
func (buildFilter) Aliases() []string { return nil }
func (buildFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}
func (buildFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}
func (buildFilter) MaskingHorizon() int { return 4096 }

func (buildFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Type == engine.EventExit {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	if ev.Type != engine.EventEOF {
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := mem.Joined()
	if strings.TrimSpace(raw) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	out, ok := compactBuildCheck(raw, "cargo build")
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}
