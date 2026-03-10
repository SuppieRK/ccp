package dockerfilters

import (
	"strconv"
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

func NewComposePSFilter() engine.ToolFilter { return composePSFilter{} }

type composePSFilter struct{}

func (composePSFilter) Tool() string      { return "docker compose ps" }
func (composePSFilter) Aliases() []string { return nil }
func (composePSFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}
func (composePSFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}
func (composePSFilter) MaskingHorizon() int { return 0 }

func (composePSFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream == engine.StderrStream {
		if ev.Type == engine.EventLine {
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		}
		return engine.Decision{Action: engine.ActionIgnore}
	}
	if ev.Type != engine.EventEOF && ev.Type != engine.EventExit {
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := mem.Joined()
	if strings.TrimSpace(raw) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	out, ok := compactComposePS(raw, defaultMaxRows)
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}

func compactComposePS(raw string, maxRows int) (string, bool) {
	lines := filtercommon.NonEmptyLines(raw)
	if len(lines) == 0 {
		return "", true
	}
	rows, ok := parseComposePSTableRows(lines)
	if !ok {
		return "", false
	}
	if len(rows) == 0 {
		return "0 services\n", true
	}
	return renderComposePSRows(rows, maxRows), true
}

func parseComposePSTableRows(lines []string) ([]composePSRow, bool) {
	headers := splitColumns(lines[0])
	if !isComposePSHeader(headers) {
		return nil, false
	}
	rows := make([]composePSRow, 0, len(lines)-1)
	for _, line := range lines[1:] {
		row, ok := parseComposePSRow(headers, line)
		if !ok {
			return nil, false
		}
		rows = append(rows, row)
	}
	return rows, true
}

func renderComposePSRows(rows []composePSRow, maxRows int) string {
	var b strings.Builder
	printed := 0
	for _, row := range rows {
		if printed >= maxRows {
			if remaining := len(rows) - printed; remaining > 0 {
				b.WriteString("... +")
				b.WriteString(strconv.Itoa(remaining))
				b.WriteString(" more\n")
			}
			break
		}
		marker := "[ok]"
		if !row.statusIsHealthy {
			marker = "[!]"
		}
		b.WriteString(marker)
		b.WriteString(" ")
		b.WriteString(row.name)
		b.WriteString(" service=")
		b.WriteString(row.service)
		b.WriteString(" image=")
		b.WriteString(shortComposePSImage(row.image))
		b.WriteString(" status=")
		b.WriteString(row.status)
		if ports := displayPorts(row.ports); ports != "-" {
			b.WriteString(" ports=")
			b.WriteString(ports)
		}
		b.WriteString("\n")
		printed++
	}
	return b.String()
}

func shortComposePSImage(image string) string {
	trimmed := strings.TrimSpace(image)
	if trimmed == "" {
		return image
	}
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 && idx+1 < len(trimmed) {
		return trimmed[idx+1:]
	}
	return trimmed
}
