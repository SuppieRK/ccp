package filters

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go-command-compression-proxy/internal/engine"
)

const (
	playwrightDispatchKey      = "playwright"
	playwrightMaxFailures      = 3
	playwrightMaxFailureDetail = 2
)

var (
	playwrightSummaryCountRe = regexp.MustCompile(`(\d+)\s+(passed|failed|flaky|skipped)`)
	playwrightDurationRe     = regexp.MustCompile(`\((\d+(?:\.\d+)?)(ms|s|m)\)`)
	playwrightFailureLineRe  = regexp.MustCompile(`^\s*\d+\)\s+(.+)$`)
	playwrightCrossLineRe    = regexp.MustCompile(`^[×✗✘]\s+(.+)$`)
)

func NewPlaywrightFilter() engine.ToolFilter { return playwrightFilter{} }

type playwrightFilter struct{}

func (playwrightFilter) Tool() string { return "playwright" }

func (playwrightFilter) Aliases() []string {
	return []string{"playwright.exe", "./playwright", "./playwright.exe", "playwright.cmd", "./playwright.cmd"}
}

func (playwrightFilter) MaskingHorizon() int { return 0 }

func (playwrightFilter) Prepare(args []string) engine.PrepareResult {
	prep := engine.PrepareResult{NormalizedArgs: append([]string{}, args...), ForcePassthrough: true}
	if !supportsPlaywrightCompaction(args) {
		return prep
	}
	prep.ForcePassthrough = false
	prep.DispatchKey = playwrightDispatchKey
	return prep
}

func supportsPlaywrightCompaction(args []string) bool {
	trimmed := nonEmptyArgs(args)
	if len(trimmed) == 0 || !strings.EqualFold(trimmed[0], "test") {
		return false
	}
	for i := 1; i < len(trimmed); i++ {
		arg := strings.ToLower(strings.TrimSpace(trimmed[i]))
		switch {
		case arg == "--reporter",
			strings.HasPrefix(arg, "--reporter="),
			arg == "--list",
			arg == "show-report",
			arg == "merge-reports",
			arg == "codegen",
			arg == "install",
			arg == "open",
			arg == "--ui",
			arg == "--trace",
			strings.HasPrefix(arg, "--trace="):
			return false
		}
	}
	return true
}

func (playwrightFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}

func (playwrightFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
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
		out, ok := compactPlaywrightOutput(raw)
		if !ok {
			return engine.Decision{Action: engine.ActionFlush, Output: raw}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: out}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

type playwrightFailure struct {
	identity string
	details  []string
}

type playwrightParse struct {
	recognized bool
	passed     int
	failed     int
	skipped    int
	flaky      int
	duration   string
	failures   []playwrightFailure
}

func compactPlaywrightOutput(raw string) (string, bool) {
	if strings.IndexByte(raw, 0) >= 0 {
		return "", false
	}
	lines := nonEmptyLines(raw)
	if len(lines) == 0 {
		return "", false
	}
	p := parsePlaywright(lines)
	if !p.recognized {
		return "", false
	}
	return renderPlaywright(p), true
}

func parsePlaywright(lines []string) playwrightParse {
	p := playwrightParse{failures: make([]playwrightFailure, 0, playwrightMaxFailures)}
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if handlePlaywrightSummaryLine(&p, trimmed) {
			continue
		}
		updatePlaywrightDuration(&p, trimmed)
		failure, nextIndex, ok := collectPlaywrightFailure(lines, i)
		if !ok {
			continue
		}
		p.recognized = true
		p.failures = appendBoundedFailures(p.failures, failure, playwrightMaxFailures)
		i = nextIndex
	}
	return p
}

func handlePlaywrightSummaryLine(p *playwrightParse, line string) bool {
	if !updatePlaywrightSummaryCounts(p, line) {
		return false
	}
	if duration := parsePlaywrightDuration(line); duration != "" {
		p.duration = duration
	}
	return true
}

func updatePlaywrightDuration(p *playwrightParse, line string) {
	if p.duration == "" {
		p.duration = parsePlaywrightDuration(line)
	}
}

func collectPlaywrightFailure(lines []string, index int) (playwrightFailure, int, bool) {
	identity, ok := parsePlaywrightFailureIdentity(strings.TrimSpace(lines[index]))
	if !ok {
		return playwrightFailure{}, index, false
	}
	failure := playwrightFailure{identity: identity}
	nextIndex := index
	for nextIndex+1 < len(lines) {
		next := strings.TrimSpace(lines[nextIndex+1])
		if next == "" || looksLikePlaywrightBoundary(next) {
			break
		}
		failure.details = appendBoundedStrings(failure.details, next, playwrightMaxFailureDetail)
		nextIndex++
	}
	return failure, nextIndex, true
}

func updatePlaywrightSummaryCounts(p *playwrightParse, line string) bool {
	matches := playwrightSummaryCountRe.FindAllStringSubmatch(strings.ToLower(line), -1)
	if len(matches) == 0 {
		return false
	}
	p.recognized = true
	for _, m := range matches {
		count := atoiPlaywright(m[1])
		switch m[2] {
		case "passed":
			p.passed = count
		case "failed":
			p.failed = count
		case "skipped":
			p.skipped = count
		case "flaky":
			p.flaky = count
		}
	}
	return true
}

func parsePlaywrightDuration(line string) string {
	m := playwrightDurationRe.FindStringSubmatch(line)
	if len(m) != 3 {
		return ""
	}
	return m[1] + m[2]
}

func parsePlaywrightFailureIdentity(line string) (string, bool) {
	if m := playwrightFailureLineRe.FindStringSubmatch(line); len(m) == 2 {
		return strings.TrimSpace(m[1]), true
	}
	if m := playwrightCrossLineRe.FindStringSubmatch(line); len(m) == 2 {
		return strings.TrimSpace(m[1]), true
	}
	return "", false
}

func looksLikePlaywrightBoundary(line string) bool {
	if _, ok := parsePlaywrightFailureIdentity(line); ok {
		return true
	}
	return playwrightSummaryCountRe.MatchString(strings.ToLower(line)) || strings.HasPrefix(line, "To open last HTML report")
}

func renderPlaywright(p playwrightParse) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "playwright: %d passed, %d failed", p.passed, p.failed)
	if p.skipped > 0 {
		_, _ = fmt.Fprintf(&b, ", %d skipped", p.skipped)
	}
	if p.flaky > 0 {
		_, _ = fmt.Fprintf(&b, ", %d flaky", p.flaky)
	}
	if p.duration != "" {
		_, _ = fmt.Fprintf(&b, " (%s)", p.duration)
	}
	b.WriteString("\n")
	if len(p.failures) > 0 {
		b.WriteString("failed tests:\n")
		for _, failure := range p.failures {
			b.WriteString("- ")
			b.WriteString(failure.identity)
			b.WriteString("\n")
			for _, detail := range failure.details {
				b.WriteString("  ")
				b.WriteString(detail)
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

func appendBoundedStrings(lines []string, line string, max int) []string {
	if len(lines) >= max {
		return lines
	}
	return append(lines, line)
}

func appendBoundedFailures(failures []playwrightFailure, failure playwrightFailure, max int) []playwrightFailure {
	if len(failures) >= max {
		return failures
	}
	return append(failures, failure)
}

func atoiPlaywright(s string) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return v
}
