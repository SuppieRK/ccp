package cargofilters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
)

func NewClippyFilter() engine.ToolFilter { return clippyFilter{} }

type clippyFilter struct{}

func (clippyFilter) Tool() string      { return "cargo clippy" }
func (clippyFilter) Aliases() []string { return nil }
func (clippyFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}
func (clippyFilter) ContextKey(ev engine.Event) string {
	// Cargo often splits diagnostic context across stdout/stderr.
	return engine.SharedContextKey(ev)
}
func (clippyFilter) MaskingHorizon() int { return 4096 }

func (clippyFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Type != engine.EventExit {
		// Shared stdout/stderr context should flush once on Exit to avoid split diagnostics.
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := mem.Joined()
	if strings.TrimSpace(raw) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	out, ok := compactClippy(raw)
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}
