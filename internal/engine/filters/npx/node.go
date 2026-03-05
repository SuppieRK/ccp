package npxfilters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

func NewNpxNodeFilter() engine.ToolFilter { return npxNodeFilter{} }

type npxNodeFilter struct{}

func (npxNodeFilter) Tool() string      { return "npx node" }
func (npxNodeFilter) Aliases() []string { return nil }
func (npxNodeFilter) MaskingHorizon() int {
	return 4096
}
func (npxNodeFilter) Prepare(args []string) engine.PrepareResult {
	normalized := append([]string{}, args...)
	return engine.PrepareResult{
		NormalizedArgs:   normalized,
		ForcePassthrough: filtercommon.NodeIsInteractiveInvocation(args),
	}
}
func (npxNodeFilter) ContextKey(ev engine.Event) string {
	return engine.SharedContextKeyForTool(ev.CommandID, "node")
}
func (npxNodeFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	switch ev.Type {
	case engine.EventTick, engine.EventEOF:
		return engine.Decision{Action: engine.ActionCollect}
	case engine.EventLine:
		if shouldIgnoreNpxNodeWrapperLine(ev.Line) {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		if filtercommon.NodeIsUnhandledFailure(ev.Line) {
			return flushCompactedNpxNodeOutput(mem.Joined(), true)
		}
		return engine.Decision{Action: engine.ActionCollect}
	case engine.EventExit:
		raw := mem.Joined()
		return flushCompactedNpxNodeOutput(raw, ev.ExitCode != 0)
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

func shouldIgnoreNpxNodeWrapperLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "need to install the following packages") ||
		strings.HasPrefix(lower, "ok to proceed?") ||
		strings.HasPrefix(lower, "npm warn exec")
}

func flushCompactedNpxNodeOutput(raw string, flushRawOnEmpty bool) engine.Decision {
	if strings.TrimSpace(raw) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	out, ok := filtercommon.NodeCompactOutput(raw)
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	if strings.TrimSpace(out) != "" {
		return engine.Decision{Action: engine.ActionFlush, Output: out}
	}
	if flushRawOnEmpty {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	return engine.Decision{Action: engine.ActionIgnore}
}
