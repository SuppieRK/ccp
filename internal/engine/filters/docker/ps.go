package dockerfilters

import (
	"fmt"
	"sort"
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

type psGroup struct {
	image  string
	status string
	ports  string
	count  int
	names  []string
}

func NewPSFilter() engine.ToolFilter { return psFilter{} }

type psFilter struct{}

func (psFilter) Tool() string      { return "docker ps" }
func (psFilter) Aliases() []string { return nil }
func (psFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}
func (psFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}
func (psFilter) MaskingHorizon() int { return 0 }

func (psFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
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
	out, ok := compactPS(raw, defaultMaxRows)
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}

func compactPS(raw string, maxRows int) (string, bool) {
	lines := filtercommon.NonEmptyLines(raw)
	if len(lines) == 0 {
		return "", true
	}
	rows, ok := parsePSTableRows(lines)
	if !ok {
		return "", false
	}
	if len(rows) == 0 {
		return "0 containers\n", true
	}
	anomalies, groups, order := groupPSRows(rows)
	return renderPSCompactedRows(len(rows), anomalies, groups, order, maxRows), true
}

func parsePSTableRows(lines []string) ([]psRow, bool) {
	headers := splitColumns(lines[0])
	if !isPSHeader(headers) {
		return nil, false
	}
	rows := make([]psRow, 0, len(lines)-1)
	for _, line := range lines[1:] {
		row, ok := parsePSRow(headers, line)
		if !ok {
			return nil, false
		}
		rows = append(rows, row)
	}
	return rows, true
}

func groupPSRows(rows []psRow) ([]psRow, map[string]*psGroup, []string) {
	anomalies := make([]psRow, 0, len(rows))
	groups := map[string]*psGroup{}
	order := make([]string, 0, len(rows))
	for _, row := range rows {
		if !row.statusIsHealthy {
			anomalies = append(anomalies, row)
			continue
		}
		key := row.image + "|" + row.status + "|" + row.portFoldKey
		g := groups[key]
		if g == nil {
			g = &psGroup{image: row.image, status: row.status, ports: row.ports}
			groups[key] = g
			order = append(order, key)
		}
		g.count++
		g.names = append(g.names, row.name)
	}
	return anomalies, groups, order
}

func renderPSCompactedRows(total int, anomalies []psRow, groups map[string]*psGroup, order []string, maxRows int) string {
	var b strings.Builder
	printed := 0
	for _, row := range anomalies {
		if reachedPSRenderLimit(&b, total, printed, maxRows) {
			return b.String()
		}
		b.WriteString(fmt.Sprintf("[!] %s %s %s status=%s ports=%s\n", row.id, row.name, row.image, row.status, displayPorts(row.ports)))
		printed++
	}
	for _, key := range order {
		if reachedPSRenderLimit(&b, total, printed, maxRows) {
			return b.String()
		}
		g := groups[key]
		sort.Strings(g.names)
		if g.count == 1 {
			b.WriteString(fmt.Sprintf("[ok] %s status=%s ports=%s name=%s\n", g.image, g.status, displayPorts(g.ports), g.names[0]))
		} else {
			b.WriteString(fmt.Sprintf("[ok x%d] %s status=%s ports=%s names=%s\n", g.count, g.image, g.status, displayPorts(g.ports), strings.Join(g.names, ",")))
		}
		printed++
	}
	return b.String()
}

func reachedPSRenderLimit(b *strings.Builder, total, printed, maxRows int) bool {
	if printed < maxRows {
		return false
	}
	if remaining := total - printed; remaining > 0 {
		b.WriteString(fmt.Sprintf("... +%d more\n", remaining))
	}
	return true
}
