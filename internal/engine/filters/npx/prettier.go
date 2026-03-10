package npxfilters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters/prettiercommon"
)

func NewNpxPrettierFilter() engine.ToolFilter { return prettierFilter{} }

type prettierFilter struct{}

func (prettierFilter) Tool() string      { return "npx prettier" }
func (prettierFilter) Aliases() []string { return nil }
func (prettierFilter) MaskingHorizon() int {
	return 0
}
func (prettierFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: append([]string{}, args...)}
}
func (prettierFilter) ContextKey(ev engine.Event) string {
	return engine.SharedContextKeyForTool(ev.CommandID, ev.Tool)
}
func (prettierFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	switch ev.Type {
	case engine.EventLine, engine.EventTick:
		return engine.Decision{Action: engine.ActionCollect}
	case engine.EventEOF:
		return engine.Decision{Action: engine.ActionIgnore}
	case engine.EventExit:
		raw := mem.Joined()
		if strings.TrimSpace(raw) == "" {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		withoutWrapper := stripNpxWrapperNoise(raw)
		if strings.TrimSpace(withoutWrapper) == "" {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		if summary, ok := prettiercommon.SummarizeOutput(withoutWrapper); ok {
			return engine.Decision{Action: engine.ActionFlush, Output: summary}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: withoutWrapper}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}
