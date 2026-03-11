package npxfilters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

func NewNpxTscFilter() engine.ToolFilter { return tscFilter{} }

type tscFilter struct{}

func (tscFilter) Tool() string      { return "npx tsc" }
func (tscFilter) Aliases() []string { return nil }
func (tscFilter) MaskingHorizon() int {
	return 0
}
func (tscFilter) Prepare(args []string) engine.PrepareResult {
	normalized := append([]string{}, args...)
	if _, specified := filtercommon.TSCPrettyMode(normalized); !specified {
		normalized = append(normalized, "--pretty", "false")
	}
	return engine.PrepareResult{NormalizedArgs: normalized}
}
func (tscFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}
func (tscFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	return processRouted(ev, mem)
}

func processRouted(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream == engine.StderrStream {
		switch ev.Type {
		case engine.EventLine:
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		default:
			return engine.Decision{Action: engine.ActionIgnore}
		}
	}
	switch ev.Type {
	case engine.EventTick, engine.EventLine:
		return engine.Decision{Action: engine.ActionCollect}
	case engine.EventEOF:
		return engine.Decision{Action: engine.ActionIgnore}
	case engine.EventExit:
		raw := mem.Joined()
		if strings.TrimSpace(raw) == "" {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		out := stripNpxWrapperNoise(raw)
		if strings.TrimSpace(out) == "" {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		if summary, ok := summarizeTSCOutput(out); ok {
			return engine.Decision{Action: engine.ActionFlush, Output: summary}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: out}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}
func summarizeTSCOutput(raw string) (string, bool) {
	return filtercommon.SummarizeTSCOutput(raw)
}
