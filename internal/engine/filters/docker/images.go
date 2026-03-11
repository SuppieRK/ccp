package dockerfilters

import (
	"fmt"
	"strconv"
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

func NewImagesFilter() engine.ToolFilter { return imagesFilter{} }

type imagesFilter struct{}

func (imagesFilter) Tool() string      { return "docker images" }
func (imagesFilter) Aliases() []string { return nil }
func (imagesFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}
func (imagesFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}
func (imagesFilter) MaskingHorizon() int { return 0 }

func (imagesFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
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
	out, ok := compactImages(raw, defaultMaxRows)
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}

func compactImages(raw string, maxRows int) (string, bool) {
	lines := filtercommon.NonEmptyLines(raw)
	if len(lines) == 0 {
		return "", true
	}
	rows, ok := parseImagesStructured(lines)
	if !ok {
		rows, ok = parseImagesTable(lines)
		if !ok {
			return "", false
		}
	}
	if len(rows) == 0 {
		return "docker images: 0 images\n", true
	}

	totalMB := 0.0
	for _, r := range rows {
		totalMB += imageSizeMB(r.size)
	}
	totalLabel := fmt.Sprintf("%.0fMB", totalMB)
	if totalMB >= 1024 {
		totalLabel = fmt.Sprintf("%.1fGB", totalMB/1024)
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "docker images: %d images (%s)\n", len(rows), totalLabel)
	limit := len(rows)
	if limit > maxRows {
		limit = maxRows
	}
	for i := 0; i < limit; i++ {
		r := rows[i]
		image := r.repo + ":" + r.tag
		if len(image) > 40 {
			image = "..." + image[len(image)-37:]
		}
		_, _ = fmt.Fprintf(&b, "%s [%s]\n", image, r.size)
	}
	if len(rows) > maxRows {
		_, _ = fmt.Fprintf(&b, "... +%d more\n", len(rows)-maxRows)
	}
	return b.String(), true
}

type imageRow struct {
	repo string
	tag  string
	size string
}

func parseImagesStructured(lines []string) ([]imageRow, bool) {
	rows := make([]imageRow, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			return nil, false
		}
		image := strings.TrimSpace(parts[0])
		size := strings.TrimSpace(parts[1])
		if image == "" || size == "" {
			return nil, false
		}
		repo, tag, ok := strings.Cut(image, ":")
		if !ok {
			return nil, false
		}
		repo = strings.TrimSpace(repo)
		tag = strings.TrimSpace(tag)
		if repo == "" || tag == "" {
			return nil, false
		}
		rows = append(rows, imageRow{repo: repo, tag: tag, size: size})
	}
	return rows, true
}

func parseImagesTable(lines []string) ([]imageRow, bool) {
	headers := splitColumns(lines[0])
	if !isImagesHeader(headers) {
		return nil, false
	}
	rows := make([]imageRow, 0, len(lines)-1)
	for _, line := range lines[1:] {
		cols := splitColumns(line)
		if len(cols) < len(headers) {
			return nil, false
		}
		repo := columnValue(headers, cols, "REPOSITORY")
		tag := columnValue(headers, cols, "TAG")
		size := columnValue(headers, cols, "SIZE")
		if repo == "" || tag == "" || size == "" {
			return nil, false
		}
		rows = append(rows, imageRow{repo: repo, tag: tag, size: size})
	}
	return rows, true
}

func imageSizeMB(v string) float64 {
	s := strings.TrimSpace(strings.ToUpper(v))
	if s == "" {
		return 0
	}
	suffix := ""
	factor := 0.0
	switch {
	case strings.HasSuffix(s, "GB"):
		suffix, factor = "GB", 1024
	case strings.HasSuffix(s, "MB"):
		suffix, factor = "MB", 1
	case strings.HasSuffix(s, "KB"):
		suffix, factor = "KB", 1.0/1024
	case strings.HasSuffix(s, "B"):
		suffix, factor = "B", 1.0/(1024*1024)
	}
	if suffix == "" {
		return 0
	}
	num := strings.TrimSpace(strings.TrimSuffix(s, suffix))
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0
	}
	return f * factor
}
