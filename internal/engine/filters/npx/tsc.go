package npxfilters

import (
	"regexp"
	"sort"
	"strings"

	"go-command-compression-proxy/internal/engine"
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
	if !hasPrettyFlag(normalized) {
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

var tscDiagRe = regexp.MustCompile(`^(.+?)\((\d+),(\d+)\):\s+(error|warning)\s+(TS\d+):\s+(.+)$`)

func hasPrettyFlag(args []string) bool {
	for _, arg := range args {
		a := strings.TrimSpace(strings.ToLower(arg))
		if a == "--pretty" || strings.HasPrefix(a, "--pretty=") {
			return true
		}
	}
	return false
}

func summarizeTSCOutput(raw string) (string, bool) {
	type diag struct {
		file     string
		line     string
		col      string
		severity string
		code     string
		msg      string
	}
	diags := make([]diag, 0, 32)
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		m := tscDiagRe.FindStringSubmatch(trimmed)
		if len(m) != 7 {
			continue
		}

		diags = append(diags, diag{
			file:     m[1],
			line:     m[2],
			col:      m[3],
			severity: m[4],
			code:     m[5],
			msg:      m[6],
		})
	}

	if len(diags) == 0 {
		return "", false
	}

	sort.Slice(diags, func(i, j int) bool {
		if diags[i].file != diags[j].file {
			return diags[i].file < diags[j].file
		}
		if diags[i].line != diags[j].line {
			return diags[i].line < diags[j].line
		}
		if diags[i].col != diags[j].col {
			return diags[i].col < diags[j].col
		}
		return diags[i].code < diags[j].code
	})

	var b strings.Builder
	lastFile := ""
	for _, d := range diags {
		if d.file != lastFile {
			b.WriteString(d.file)
			b.WriteString(":\n")
			lastFile = d.file
		}
		b.WriteString("- ")
		b.WriteString(d.line)
		b.WriteString(":")
		b.WriteString(d.col)
		b.WriteString(" ")
		b.WriteString(d.severity)
		b.WriteString(" ")
		b.WriteString(d.code)
		b.WriteString(" ")
		b.WriteString(d.msg)
		b.WriteString("\n")
	}
	return b.String(), true
}
