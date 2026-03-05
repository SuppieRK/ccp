package gitfilters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
)

// NewGitPullFilter compacts git pull write-path output.
func NewGitPullFilter() engine.ToolFilter { return gitPullFilter{} }

type gitPullFilter struct{}

func (gitPullFilter) Tool() string        { return "git pull" }
func (gitPullFilter) Aliases() []string   { return nil }
func (gitPullFilter) MaskingHorizon() int { return 0 }
func (gitPullFilter) ContextKey(ev engine.Event) string {
	return sharedContextKey(ev)
}

func (gitPullFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}

func (gitPullFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	return processExit(ev, mem, func(raw string) string {
		lower := strings.ToLower(raw)
		if strings.Contains(lower, "already up to date") || strings.Contains(lower, "already up-to-date") {
			return "Up-to-date\n"
		}
		return "OK\n"
	})
}
