package cargofilters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
)

func NewCheckFilter() engine.ToolFilter { return checkFilter{} }

type checkFilter struct{}

func (checkFilter) Tool() string      { return "cargo check" }
func (checkFilter) Aliases() []string { return nil }
func (checkFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}
func (checkFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}
func (checkFilter) MaskingHorizon() int { return 4096 }

func (checkFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Type == engine.EventExit {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	if ev.Type != engine.EventEOF {
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := mem.Joined()
	if strings.TrimSpace(raw) == "" {
		if ev.Stream == engine.StderrStream {
			return engine.Decision{Action: engine.ActionFlush, Output: "cargo check: ok\n"}
		}
		return engine.Decision{Action: engine.ActionIgnore}
	}
	out, ok := compactBuildCheck(raw, "cargo check")
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}
