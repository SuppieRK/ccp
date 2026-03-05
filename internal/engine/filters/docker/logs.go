package dockerfilters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

func NewLogsFilter() engine.ToolFilter { return logsFilter{} }

type logsFilter struct{}

func (logsFilter) Tool() string      { return "docker logs" }
func (logsFilter) Aliases() []string { return nil }
func (logsFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}
func (logsFilter) ContextKey(ev engine.Event) string {
	container := filtercommon.DispatchValue(ev.Dispatch, "container")
	if container == "" {
		container = "unknown"
	}
	return ev.CommandID + ":" + ev.Tool + ":" + container + ":" + string(ev.Stream)
}
func (logsFilter) MaskingHorizon() int { return 4096 }

func (logsFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream == engine.StderrStream {
		if ev.Type == engine.EventLine {
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		}
		return engine.Decision{Action: engine.ActionIgnore}
	}
	switch ev.Type {
	case engine.EventLine, engine.EventTick:
		return engine.Decision{Action: engine.ActionCollect}
	case engine.EventEOF, engine.EventExit:
		raw := mem.Joined()
		if strings.TrimSpace(raw) == "" {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}
