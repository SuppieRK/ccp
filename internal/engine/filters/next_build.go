package filters

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go-command-compression-proxy/internal/engine"
)

const (
	nextBuildDispatchKey = "next-build"
	nextBuildMaxBundles  = 5
	nextBuildMaxDetails  = 5
	nextBuildMaxWarnings = 3
)

var (
	nextBuildANSIEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	nextBuildRouteLineRe  = regexp.MustCompile(`^[\s│├└┌─]*([○●◐λƒ])\s+(\S+)\s+(\d+(?:\.\d+)?)\s*(B|kB|MB)\s+(\d+(?:\.\d+)?)\s*(B|kB|MB)`)
	nextBuildTimeRe       = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(ms|s)`)
)

func NewNextBuildFilter() engine.ToolFilter { return nextBuildFilter{} }

type nextBuildFilter struct{}

func (nextBuildFilter) Tool() string { return "next" }

func (nextBuildFilter) Aliases() []string {
	return []string{"next.exe", "./next", "./next.exe", "next.cmd", "./next.cmd"}
}

func (nextBuildFilter) MaskingHorizon() int { return 0 }

func (nextBuildFilter) Prepare(args []string) engine.PrepareResult {
	prep := engine.PrepareResult{NormalizedArgs: append([]string{}, args...), ForcePassthrough: true}
	if !supportsNextBuildShape(args) {
		return prep
	}
	prep.ForcePassthrough = false
	prep.DispatchKey = nextBuildDispatchKey
	return prep
}

func supportsNextBuildShape(args []string) bool {
	trimmed := nonEmptyArgs(args)
	if len(trimmed) == 0 || !strings.EqualFold(trimmed[0], "build") {
		return false
	}
	for _, arg := range trimmed[1:] {
		lower := strings.ToLower(strings.TrimSpace(arg))
		switch {
		case lower == "--debug",
			lower == "--profile",
			lower == "--help",
			lower == "-h",
			lower == "--experimental-debug-memory-usage",
			strings.HasPrefix(lower, "--experimental-build-mode"),
			strings.HasPrefix(lower, "--debug-prerender"):
			return false
		}
	}
	return true
}

func (nextBuildFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}

func (nextBuildFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream == engine.StderrStream {
		switch ev.Type {
		case engine.EventLine:
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		default:
			return engine.Decision{Action: engine.ActionIgnore}
		}
	}
	switch ev.Type {
	case engine.EventLine, engine.EventTick:
		return engine.Decision{Action: engine.ActionCollect}
	case engine.EventEOF:
		return engine.Decision{Action: engine.ActionIgnore}
	case engine.EventExit:
		raw := mem.Joined()
		if strings.TrimSpace(raw) == "" {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		if out, ok := summarizeNextBuild(raw); ok {
			return engine.Decision{Action: engine.ActionFlush, Output: out}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

type nextBuildRoute struct {
	kind      string
	route     string
	firstLoad float64
	unit      string
}

type nextBuildSummary struct {
	routes      []nextBuildRoute
	diagnostics []string
	warnings    []string
	buildTime   string
	status      string
	recognized  bool
}

func summarizeNextBuild(raw string) (string, bool) {
	summary := parseNextBuildSummary(raw)
	if !summary.recognized {
		return "", false
	}
	return renderNextBuildSummary(summary), true
}

func parseNextBuildSummary(raw string) nextBuildSummary {
	clean := nextBuildANSIEscapeRe.ReplaceAllString(strings.ReplaceAll(raw, "\r", ""), "")
	var summary nextBuildSummary
	for _, rawLine := range strings.Split(clean, "\n") {
		if nextBuildShouldSkipLine(rawLine) {
			continue
		}
		updateNextBuildSummaryLine(&summary, strings.TrimSpace(rawLine))
	}
	if summary.status == "" {
		summary.status = nextBuildDefaultStatus(summary)
	}
	sort.Slice(summary.routes, func(i, j int) bool {
		return summary.routes[i].firstLoad > summary.routes[j].firstLoad
	})
	return summary
}

func nextBuildShouldSkipLine(rawLine string) bool {
	return strings.TrimSpace(rawLine) == ""
}

func updateNextBuildSummaryLine(summary *nextBuildSummary, line string) bool {
	if route, ok := parseNextBuildRoute(line); ok {
		summary.routes = append(summary.routes, route)
		summary.recognized = true
		return true
	}
	updateNextBuildStatus(summary, line)
	if nextBuildIsFailure(line) {
		summary.diagnostics = appendBounded(summary.diagnostics, line, nextBuildMaxDetails)
		return true
	}
	if nextBuildIsDiagnostic(line) {
		summary.diagnostics = appendBounded(summary.diagnostics, line, nextBuildMaxDetails)
		summary.recognized = true
	}
	if nextBuildIsWarning(line) {
		summary.warnings = appendBounded(summary.warnings, line, nextBuildMaxWarnings)
		summary.recognized = true
	}
	if summary.buildTime == "" {
		summary.buildTime = nextBuildExtractTime(line)
	}
	return false
}

func updateNextBuildStatus(summary *nextBuildSummary, line string) {
	if nextBuildIsCached(line) {
		summary.status = "cached"
		summary.recognized = true
	}
	if nextBuildIsSuccess(line) {
		summary.status = "success"
		summary.recognized = true
	}
	if nextBuildIsFailure(line) {
		summary.status = "failed"
		summary.recognized = true
	}
}

func parseNextBuildRoute(line string) (nextBuildRoute, bool) {
	m := nextBuildRouteLineRe.FindStringSubmatch(line)
	if len(m) != 7 {
		return nextBuildRoute{}, false
	}
	load, err := strconv.ParseFloat(m[5], 64)
	if err != nil {
		return nextBuildRoute{}, false
	}
	return nextBuildRoute{kind: m[1], route: m[2], firstLoad: load, unit: m[6]}, true
}

func nextBuildIsCached(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "already optimized") || strings.Contains(lower, "using cache") || strings.Contains(lower, "cache hit")
}

func nextBuildIsSuccess(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "compiled successfully") || strings.Contains(lower, "built in") || strings.Contains(lower, "collecting page data") || strings.Contains(lower, "finalizing page optimization")
}

func nextBuildIsFailure(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "failed to compile") || strings.Contains(lower, "build error occurred")
}

func nextBuildIsDiagnostic(line string) bool {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "error:") {
		return true
	}
	return strings.HasPrefix(line, "./") || strings.HasPrefix(line, "src/") || strings.HasPrefix(line, "app/")
}

func nextBuildIsWarning(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "warning") || strings.Contains(lower, "warn ") || strings.HasPrefix(lower, "warn")
}

func nextBuildExtractTime(line string) string {
	m := nextBuildTimeRe.FindStringSubmatch(line)
	if len(m) != 3 {
		return ""
	}
	return m[1] + m[2]
}

func nextBuildDefaultStatus(summary nextBuildSummary) string {
	if len(summary.diagnostics) > 0 {
		return "failed"
	}
	if len(summary.routes) > 0 || summary.buildTime != "" {
		return "success"
	}
	return ""
}

func renderNextBuildSummary(summary nextBuildSummary) string {
	var b strings.Builder
	b.WriteString("next build: ")
	if summary.status == "" {
		b.WriteString("summary")
	} else {
		b.WriteString(summary.status)
	}
	b.WriteString("\n")
	if summary.buildTime != "" {
		_, _ = fmt.Fprintf(&b, "time: %s\n", summary.buildTime)
	}
	if len(summary.routes) > 0 {
		writeNextBuildRoutes(&b, summary.routes)
	}
	if len(summary.warnings) > 0 {
		b.WriteString("warnings:\n")
		for _, line := range summary.warnings {
			_, _ = fmt.Fprintf(&b, "- %s\n", line)
		}
	}
	if len(summary.diagnostics) > 0 {
		b.WriteString("details:\n")
		for _, line := range summary.diagnostics {
			_, _ = fmt.Fprintf(&b, "- %s\n", line)
		}
	}
	return b.String()
}

func writeNextBuildRoutes(b *strings.Builder, routes []nextBuildRoute) {
	staticCount, dynamicCount, serverCount := 0, 0, 0
	for _, route := range routes {
		switch route.kind {
		case "○":
			staticCount++
		case "●", "◐":
			dynamicCount++
		case "λ", "ƒ":
			serverCount++
		}
	}
	_, _ = fmt.Fprintf(b, "routes: %d total (%d static, %d dynamic, %d server)\n", len(routes), staticCount, dynamicCount, serverCount)
	b.WriteString("top bundles:\n")
	for i, route := range routes {
		if i >= nextBuildMaxBundles {
			_, _ = fmt.Fprintf(b, "+ %d more routes\n", len(routes)-i)
			break
		}
		_, _ = fmt.Fprintf(b, "- %s %s %.1f %s\n", nextBuildRouteKind(route.kind), route.route, route.firstLoad, route.unit)
	}
}

func nextBuildRouteKind(kind string) string {
	switch kind {
	case "○":
		return "static"
	case "●", "◐":
		return "dynamic"
	case "λ", "ƒ":
		return "server"
	default:
		return "route"
	}
}

func appendBounded(lines []string, line string, max int) []string {
	if len(lines) >= max {
		return lines
	}
	return append(lines, line)
}
