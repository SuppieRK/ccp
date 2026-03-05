package gitfilters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
)

// NewGitLogFilter compacts git log output.
func NewGitLogFilter() engine.ToolFilter { return gitLogFilter{} }

type gitLogFilter struct{}

func (gitLogFilter) Tool() string        { return "git log" }
func (gitLogFilter) Aliases() []string   { return nil }
func (gitLogFilter) MaskingHorizon() int { return 0 }
func (gitLogFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}
func (gitLogFilter) Prepare(args []string) engine.PrepareResult {
	hasFormat := false
	hasLimit := false
	wantsMerges := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "--oneline") ||
			strings.HasPrefix(arg, "--pretty") ||
			strings.HasPrefix(arg, "--format") {
			hasFormat = true
		}
		if strings.HasPrefix(arg, "-") && len(arg) > 1 && arg[1] >= '0' && arg[1] <= '9' {
			hasLimit = true
		}
		if arg == "--merges" || arg == "--min-parents=2" {
			wantsMerges = true
		}
	}
	normalized := make([]string, 0, len(args)+3)
	if !hasFormat {
		normalized = append(normalized, "--pretty=format:%h %aI %an <%ae> | %s")
	}
	if !hasLimit {
		normalized = append(normalized, "-10")
	}
	if !wantsMerges {
		normalized = append(normalized, "--no-merges")
	}
	normalized = append(normalized, args...)
	return engine.PrepareResult{NormalizedArgs: normalized}
}

func (gitLogFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream == engine.StderrStream {
		if ev.Type == engine.EventLine {
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		}
		return engine.Decision{Action: engine.ActionIgnore}
	}
	if ev.Type != engine.EventEOF {
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := strings.TrimSpace(mem.Joined())
	if raw == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		if len(line) > 120 {
			lines[i] = line[:117] + "..."
		}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: strings.Join(lines, "\n") + "\n"}
}
