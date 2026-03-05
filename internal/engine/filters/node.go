package filters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

// NewNodeFilter returns the built-in node tool filter.
func NewNodeFilter() engine.ToolFilter { return nodeFilter{} }

type nodeFilter struct{}

func (nodeFilter) Tool() string { return "node" }

func (nodeFilter) Aliases() []string {
	return []string{"node.exe", "./node.exe", "node.cmd", "./node.cmd"}
}

func (nodeFilter) Prepare(args []string) engine.PrepareResult {
	normalized := append([]string{}, args...)
	if filtercommon.NodeIsInteractiveInvocation(args) {
		return engine.PrepareResult{NormalizedArgs: normalized, ForcePassthrough: true}
	}
	return engine.PrepareResult{
		NormalizedArgs: normalized,
		DispatchKey:    "node|mode=runtime",
	}
}

func (nodeFilter) ContextKey(ev engine.Event) string {
	// Node runtime errors can split across streams; use one shared context.
	return engine.SharedContextKey(ev)
}

func (nodeFilter) MaskingHorizon() int { return 4096 }

func (nodeFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Type == engine.EventLine {
		return handleNodeLine(ev, mem)
	}
	if ev.Type == engine.EventExit {
		return handleNodeExit(ev, mem.Joined())
	}
	return engine.Decision{Action: engine.ActionCollect}
}

func handleNodeLine(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if !filtercommon.NodeIsUnhandledFailure(ev.Line) {
		return engine.Decision{Action: engine.ActionCollect}
	}
	if mem.Len() <= 1 {
		return engine.Decision{Action: engine.ActionFlush, Output: ev.Line}
	}
	raw := mem.Joined()
	out, ok := compactNodeOutput(raw)
	if !ok || strings.TrimSpace(out) == "" {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	// Flush buffered warning noise first; include the failure line in the same emission.
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}

func handleNodeExit(ev engine.Event, raw string) engine.Decision {
	if strings.TrimSpace(raw) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	out, ok := compactNodeOutput(raw)
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	if strings.TrimSpace(out) == "" {
		if ev.ExitCode != 0 {
			return engine.Decision{Action: engine.ActionFlush, Output: raw}
		}
		return engine.Decision{Action: engine.ActionIgnore}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}

func compactNodeOutput(raw string) (string, bool) {
	return filtercommon.NodeCompactOutput(raw)
}
