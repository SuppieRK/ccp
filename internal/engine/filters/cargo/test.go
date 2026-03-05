package cargofilters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
)

func NewTestFilter() engine.ToolFilter { return testFilter{} }

type testFilter struct{}

func (testFilter) Tool() string      { return "cargo test" }
func (testFilter) Aliases() []string { return nil }
func (testFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}
func (testFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}
func (testFilter) MaskingHorizon() int { return 4096 }

func (testFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Type == engine.EventExit {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	if ev.Type != engine.EventEOF {
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := mem.Joined()
	if raw = strings.TrimSpace(raw); raw == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	out, ok := compactTest(raw)
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	if out = strings.TrimSpace(out); out == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}
