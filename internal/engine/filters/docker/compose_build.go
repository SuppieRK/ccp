package dockerfilters

import (
	"sort"
	"strconv"
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

func NewComposeBuildFilter() engine.ToolFilter { return composeBuildFilter{} }

type composeBuildFilter struct{}

func (composeBuildFilter) Tool() string      { return "docker compose build" }
func (composeBuildFilter) Aliases() []string { return nil }
func (composeBuildFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}
func (composeBuildFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}
func (composeBuildFilter) MaskingHorizon() int { return 0 }

func (composeBuildFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
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
	out, ok := compactComposeBuild(raw, defaultMaxRows)
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}

type composeBuildServiceSummary struct {
	name  string
	steps int
	built bool
}

type composeBuildScanResult struct {
	serviceSteps      map[string]int
	builtTargets      []string
	finishedServices  map[string]struct{}
	sawServiceMarkers bool
}

func compactComposeBuild(raw string, maxRows int) (string, bool) {
	lines := filtercommon.NonEmptyLines(raw)
	if len(lines) == 0 {
		return "", true
	}
	scan, ok := scanComposeBuildLines(lines)
	if !ok || !scan.sawServiceMarkers {
		return "", false
	}
	summaries := composeBuildSummaries(scan)
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].name < summaries[j].name })
	return renderComposeBuildSummaries(summaries, maxRows), true
}

func scanComposeBuildLines(lines []string) (composeBuildScanResult, bool) {
	scan := composeBuildScanResult{
		serviceSteps:     map[string]int{},
		builtTargets:     make([]string, 0, 4),
		finishedServices: map[string]struct{}{},
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if composeBuildLineIsFailure(trimmed) {
			return composeBuildScanResult{}, false
		}
		if target, ok := composeBuildTargetFromLine(trimmed); ok {
			scan.builtTargets = append(scan.builtTargets, target)
		}
		if !composeBuildCaptureServiceProgress(trimmed, &scan) {
			continue
		}
		scan.sawServiceMarkers = true
	}
	return scan, true
}

func composeBuildLineIsFailure(line string) bool {
	return strings.Contains(line, "failed to solve") || strings.Contains(line, "ERROR")
}

func composeBuildCaptureServiceProgress(line string, scan *composeBuildScanResult) bool {
	svc, ok := composeBuildServiceFromBracket(line)
	if !ok {
		return false
	}
	scan.serviceSteps[svc]++
	if strings.Contains(line, "exporting to image") || strings.Contains(line, "exporting manifest") {
		scan.finishedServices[svc] = struct{}{}
	}
	return true
}

func composeBuildSummaries(scan composeBuildScanResult) []composeBuildServiceSummary {
	summaries := make([]composeBuildServiceSummary, 0, len(scan.serviceSteps))
	for svc, steps := range scan.serviceSteps {
		_, built := composeBuildTargetForService(svc, scan.builtTargets)
		if _, ok := scan.finishedServices[svc]; ok {
			built = true
		}
		summaries = append(summaries, composeBuildServiceSummary{name: svc, steps: steps, built: built})
	}
	return summaries
}

func composeBuildServiceFromBracket(line string) (string, bool) {
	start := strings.Index(line, "[")
	end := strings.Index(line, "]")
	if start < 0 || end <= start+1 {
		return "", false
	}
	content := strings.TrimSpace(line[start+1 : end])
	if content == "" || strings.HasPrefix(content, "+") {
		return "", false
	}
	service := strings.Fields(content)[0]
	if service == "" || strings.EqualFold(service, "internal") {
		return "", false
	}
	return service, true
}

func composeBuildTargetFromLine(line string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 2 || fields[len(fields)-1] != "Built" {
		return "", false
	}
	return fields[0], true
}

func composeBuildTargetForService(service string, targets []string) (string, bool) {
	for _, target := range targets {
		if target == service || strings.HasSuffix(target, "-"+service) {
			return target, true
		}
	}
	return "", false
}

func renderComposeBuildSummaries(summaries []composeBuildServiceSummary, maxRows int) string {
	var b strings.Builder
	printed := 0
	for _, summary := range summaries {
		if printed >= maxRows {
			if remaining := len(summaries) - printed; remaining > 0 {
				b.WriteString("... +")
				b.WriteString(strconv.Itoa(remaining))
				b.WriteString(" more\n")
			}
			break
		}
		marker := "[build]"
		if summary.built {
			marker = "[ok]"
		}
		b.WriteString(marker)
		b.WriteString(" ")
		b.WriteString(summary.name)
		if summary.built {
			b.WriteString(" built")
		} else {
			b.WriteString(" steps=")
			b.WriteString(strconv.Itoa(summary.steps))
		}
		b.WriteString("\n")
		printed++
	}
	return b.String()
}
