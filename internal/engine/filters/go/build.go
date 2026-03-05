package gofilters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

func NewBuildFilter() engine.ToolFilter { return buildFilter{} }

type buildFilter struct{}

func (buildFilter) Tool() string      { return "go build" }
func (buildFilter) Aliases() []string { return nil }
func (buildFilter) Prepare(args []string) engine.PrepareResult {
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if strings.EqualFold(trimmed, "-json") || strings.HasPrefix(strings.ToLower(trimmed), "--json") {
			return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true, Ambiguous: true, Reason: "structured output mode"}
		}
	}
	return engine.PrepareResult{NormalizedArgs: args}
}
func (buildFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}
func (buildFilter) MaskingHorizon() int { return 0 }

func (buildFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream != engine.StderrStream {
		if ev.Type != engine.EventExit {
			return engine.Decision{Action: engine.ActionCollect}
		}
	} else {
		if filtercommon.DispatchValue(ev.Dispatch, "x") != "1" {
			if ev.Type != engine.EventLine {
				return engine.Decision{Action: engine.ActionIgnore}
			}
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		}
		if ev.Type == engine.EventExit {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		if ev.Type != engine.EventEOF {
			return engine.Decision{Action: engine.ActionCollect}
		}
	}
	raw := mem.Joined()
	if strings.TrimSpace(raw) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	out, ok := compactBuildVet(raw, "go build")
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}
