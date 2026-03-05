package npxfilters

import (
	"regexp"
	"strings"

	"go-command-compression-proxy/internal/engine"
)

func NewNpxPrismaFilter() engine.ToolFilter { return prismaFilter{} }

type prismaFilter struct{}

func (prismaFilter) Tool() string      { return "npx prisma" }
func (prismaFilter) Aliases() []string { return nil }
func (prismaFilter) MaskingHorizon() int {
	return 0
}
func (prismaFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: append([]string{}, args...)}
}
func (prismaFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}
func (prismaFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream == engine.StderrStream {
		if ev.Type == engine.EventLine {
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		}
		return engine.Decision{Action: engine.ActionIgnore}
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
		// Preserve full diagnostics on failures.
		if ev.ExitCode != 0 {
			return engine.Decision{Action: engine.ActionFlush, Output: out}
		}
		if summary, ok := summarizePrismaSuccess(out); ok {
			return engine.Decision{Action: engine.ActionFlush, Output: summary}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: out}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

var prismaFormattedPathRe = regexp.MustCompile(`(?i)^formatted\s+(.+?)\s+in\s+\d+(\.\d+)?ms`)

func summarizePrismaSuccess(raw string) (string, bool) {
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "schema") && strings.Contains(lower, "is valid") {
			return "prisma validate: ok\n", true
		}
		m := prismaFormattedPathRe.FindStringSubmatch(trimmed)
		if len(m) > 1 {
			return "prisma format: ok " + strings.TrimSpace(m[1]) + "\n", true
		}
	}
	return "", false
}
