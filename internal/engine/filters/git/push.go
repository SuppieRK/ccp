package gitfilters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
)

// NewGitPushFilter compacts git push write-path output.
func NewGitPushFilter() engine.ToolFilter { return gitPushFilter{} }

type gitPushFilter struct{}

func (gitPushFilter) Tool() string        { return "git push" }
func (gitPushFilter) Aliases() []string   { return nil }
func (gitPushFilter) MaskingHorizon() int { return 0 }
func (gitPushFilter) ContextKey(ev engine.Event) string {
	return sharedContextKey(ev)
}

func (gitPushFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}

func (gitPushFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	return processExit(ev, mem, func(raw string) string {
		if strings.Contains(strings.ToLower(strings.TrimSpace(raw)), "everything up-to-date") {
			return "Up-to-date\n"
		}
		return "OK\n"
	})
}
