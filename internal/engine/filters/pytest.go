package filters

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

// NewPytestFilter returns pytest semantic compaction filter.
func NewPytestFilter() engine.ToolFilter { return pytestFilter{} }

type pytestFilter struct{}

func (pytestFilter) Tool() string { return "pytest" }

func (pytestFilter) Aliases() []string {
	return []string{"pytest.exe", "./pytest.exe", "pytest.cmd", "./pytest.cmd"}
}

func (pytestFilter) Prepare(args []string) engine.PrepareResult {
	normalized := filtercommon.CopyArgs(args)
	hasTraceback := false
	hasVerbose := false
	hasHeader := false
	for _, arg := range normalized {
		v := filtercommon.LowerTrim(arg)
		if v == "--tb" || strings.HasPrefix(v, "--tb=") {
			hasTraceback = true
		}
		if v == "--verbose" || pytestVerboseShortRe.MatchString(v) {
			hasVerbose = true
		}
		if v == "--no-header" || v == "--header" {
			hasHeader = true
		}
	}
	// Honor explicit troubleshooting intent exactly as provided.
	if hasTraceback || hasVerbose {
		return engine.PrepareResult{NormalizedArgs: normalized}
	}
	// Apply compact defaults only when equivalent controls are absent.
	normalized = append(normalized, "--tb=short")
	if !hasHeader {
		normalized = append(normalized, "--no-header")
	}
	return engine.PrepareResult{NormalizedArgs: normalized}
}

func (pytestFilter) ContextKey(ev engine.Event) string {
	// Keep stderr passthrough isolated from stdout semantic parsing.
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}

func (pytestFilter) MaskingHorizon() int { return 0 }

func (pytestFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if d, ok := stderrImmediateOrIgnore(ev, nil); ok {
		return d
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
		out, ok := compactPytestOutput(raw)
		if !ok {
			return engine.Decision{Action: engine.ActionFlush, Output: raw}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: out}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

var pytestVerboseShortRe = regexp.MustCompile(`^-v+$`)

var (
	pytestSummaryCountRe = regexp.MustCompile(`(\d+)\s+(passed|failed|skipped|error|errors|xfailed|xpassed)`)
	pytestFailureHdrRe   = regexp.MustCompile(`^_{3,}\s*(.*?)\s*_{3,}$`)
	pytestShortFailRe    = regexp.MustCompile(`^(FAILED|ERROR)\s+(.+)$`)
	pytestSourceLineRe   = regexp.MustCompile(`^\s*>?\s*\d+\s+.*$`)
	pytestCollectedRe    = regexp.MustCompile(`^collected\s+(\d+)\s+items?$`)
)

type pytestFailureDetail struct {
	nodeID   string
	message  string
	context  []string
	captured []string
}

type pytestParse struct {
	recognized  bool
	passed      int
	failed      int
	skipped     int
	errors      int
	noTests     bool
	shortFailed []string
	details     []pytestFailureDetail
}

type pytestParseState int

const (
	pytestStateDefault pytestParseState = iota
	pytestStateFailures
	pytestStateShortSummary
)

func compactPytestOutput(raw string) (string, bool) {
	if strings.ContainsRune(raw, '\x00') {
		return "", false
	}
	lines := filtercommon.NonEmptyLines(raw)
	if len(lines) == 0 {
		return "", false
	}
	p := parsePytest(lines)
	if !p.recognized {
		return "", false
	}
	return renderPytestCompaction(p), true
}

func renderPytestCompaction(p pytestParse) string {
	if p.noTests {
		return "pytest: no tests collected\n"
	}
	if p.failed == 0 && p.errors == 0 {
		return renderPytestPassOnlySummary(p.passed, p.skipped)
	}
	return renderPytestFailureSummary(p)
}

func renderPytestPassOnlySummary(passed, skipped int) string {
	if passed == 0 {
		return "pytest: complete\n"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("pytest: %d passed", passed))
	if skipped > 0 {
		b.WriteString(fmt.Sprintf(", %d skipped", skipped))
	}
	b.WriteString("\n")
	return b.String()
}

func renderPytestFailureSummary(p pytestParse) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("pytest: %d passed, %d failed", p.passed, p.failed))
	if p.errors > 0 {
		b.WriteString(fmt.Sprintf(", %d errors", p.errors))
	}
	if p.skipped > 0 {
		b.WriteString(fmt.Sprintf(", %d skipped", p.skipped))
	}
	b.WriteString("\n")

	writePytestFailureDetails(&b, p.details)
	writePytestFailedTests(&b, p.shortFailed)
	return b.String()
}

func writePytestFailureDetails(b *strings.Builder, details []pytestFailureDetail) {
	if len(details) == 0 {
		return
	}
	b.WriteString("failure details:\n")
	for i, d := range details {
		if i >= 3 {
			break
		}
		b.WriteString("- ")
		b.WriteString(d.nodeID)
		if d.message != "" {
			b.WriteString(": ")
			b.WriteString(d.message)
		}
		b.WriteString("\n")
		for _, line := range d.context {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
		for _, line := range d.captured {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
}

func writePytestFailedTests(b *strings.Builder, shortFailed []string) {
	if len(shortFailed) == 0 {
		return
	}
	b.WriteString("failed tests:\n")
	for _, failed := range shortFailed {
		b.WriteString("- ")
		b.WriteString(failed)
		b.WriteString("\n")
	}
}

func parsePytest(lines []string) pytestParse {
	p := pytestParse{
		shortFailed: make([]string, 0, 8),
		details:     make([]pytestFailureDetail, 0, 8),
	}
	curState := pytestStateDefault
	curHeader := ""
	curBlock := make([]string, 0, 16)
	flushBlock := func() {
		if strings.TrimSpace(curHeader) == "" && len(curBlock) == 0 {
			return
		}
		d := pytestFailureDetail{
			nodeID: strings.TrimSpace(curHeader),
		}
		if d.nodeID == "" {
			d.nodeID = "unknown-test"
		}
		d.message = extractPytestFailureMessage(curBlock)
		d.context = extractPytestFailureContext(curBlock)
		d.captured = extractPytestCapturedOutput(curBlock)
		p.details = append(p.details, d)
		curHeader = ""
		curBlock = curBlock[:0]
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		consumePytestGlobalSignals(lower, &p)
		nextState, switched := nextPytestState(trimmed, lower, curState)
		if switched {
			if nextState == pytestStateShortSummary {
				flushBlock()
			}
			curState = nextState
			p.recognized = true
			continue
		}
		consumePytestSummaryCounts(lower, &p)
		consumePytestStateLine(trimmed, line, curState, &curHeader, &curBlock, &p, flushBlock)
	}
	flushBlock()
	return p
}

func consumePytestGlobalSignals(lower string, p *pytestParse) {
	if m := pytestCollectedRe.FindStringSubmatch(lower); len(m) == 2 {
		total, _ := strconv.Atoi(m[1])
		if total == 0 {
			p.noTests = true
		}
		p.recognized = true
	}
	if strings.Contains(lower, "no tests ran") || strings.Contains(lower, "no tests collected") {
		p.noTests = true
		p.recognized = true
	}
}

func nextPytestState(trimmed, lower string, curState pytestParseState) (pytestParseState, bool) {
	if strings.HasPrefix(trimmed, "===") && strings.Contains(lower, "failures") {
		return pytestStateFailures, curState != pytestStateFailures
	}
	if strings.HasPrefix(trimmed, "===") && strings.Contains(lower, "short test summary info") {
		return pytestStateShortSummary, curState != pytestStateShortSummary
	}
	return curState, false
}

func consumePytestSummaryCounts(lower string, p *pytestParse) {
	for _, m := range pytestSummaryCountRe.FindAllStringSubmatch(lower, -1) {
		n, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "passed":
			p.passed = n
		case "failed":
			p.failed = n
		case "skipped":
			p.skipped = n
		case "error", "errors":
			p.errors = n
		}
		p.recognized = true
	}
}

func consumePytestStateLine(trimmed, raw string, curState pytestParseState, curHeader *string, curBlock *[]string, p *pytestParse, flushBlock func()) {
	switch curState {
	case pytestStateDefault:
		// No section-specific handling in default state.
	case pytestStateFailures:
		if m := pytestFailureHdrRe.FindStringSubmatch(trimmed); len(m) == 2 {
			flushBlock()
			*curHeader = strings.TrimSpace(m[1])
			return
		}
		if strings.HasPrefix(trimmed, "===") {
			return
		}
		if trimmed != "" {
			*curBlock = append(*curBlock, raw)
		}
	case pytestStateShortSummary:
		if m := pytestShortFailRe.FindStringSubmatch(trimmed); len(m) == 3 {
			p.shortFailed = append(p.shortFailed, strings.TrimSpace(m[2]))
			p.recognized = true
		}
	default:
		// Preserve forward compatibility for unknown parser states.
	}
}

func extractPytestFailureMessage(lines []string) string {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "E   ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "E   "))
		}
		if strings.Contains(trimmed, "AssertionError") || strings.Contains(trimmed, "Error:") {
			return trimmed
		}
	}
	return ""
}

func extractPytestFailureContext(lines []string) []string {
	src := make([]string, 0, 16)
	failIdx := -1
	for _, line := range lines {
		if !pytestSourceLineRe.MatchString(line) {
			continue
		}
		n := strings.TrimSpace(line)
		if strings.HasPrefix(n, ">") {
			failIdx = len(src)
		}
		src = append(src, n)
	}
	if len(src) == 0 {
		return nil
	}
	if failIdx < 0 {
		failIdx = len(src) / 2
	}
	start := failIdx - 3
	if start < 0 {
		start = 0
	}
	end := failIdx + 3
	if end >= len(src) {
		end = len(src) - 1
	}
	return append([]string{}, src[start:end+1]...)
}

func extractPytestCapturedOutput(lines []string) []string {
	captured := make([]string, 0, 12)
	captureActive := false
	captureBudget := 12
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "captured stdout") || strings.Contains(lower, "captured stderr") {
			captureActive = true
			captured = append(captured, trimmed)
			continue
		}
		if !captureActive {
			continue
		}
		if trimmed == "" {
			captureActive = false
			continue
		}
		if strings.HasPrefix(trimmed, "E   ") || captureBudget <= 0 {
			continue
		}
		captured = append(captured, trimmed)
		captureBudget--
	}
	return captured
}
