package gitfilters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
)

// NewGitStatusFilter enforces porcelain output shape and forwards output without extra compaction.
func NewGitStatusFilter() engine.ToolFilter { return gitStatusFilter{} }

type gitStatusFilter struct{}

func (gitStatusFilter) Tool() string        { return "git status" }
func (gitStatusFilter) Aliases() []string   { return nil }
func (gitStatusFilter) MaskingHorizon() int { return 0 }
func (gitStatusFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}
func (gitStatusFilter) Prepare(args []string) engine.PrepareResult {
	for _, arg := range args {
		if arg == "--porcelain" || strings.HasPrefix(arg, "--porcelain=") {
			return engine.PrepareResult{NormalizedArgs: args}
		}
	}
	normalized := append([]string{}, args...)
	normalized = append(normalized, "--porcelain")
	return engine.PrepareResult{NormalizedArgs: normalized}
}

func (gitStatusFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream == engine.StderrStream {
		switch ev.Type {
		case engine.EventLine:
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		default:
			return engine.Decision{Action: engine.ActionIgnore}
		}
	}
	if ev.Type != engine.EventEOF {
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := mem.Joined()
	if raw == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: raw}
}
