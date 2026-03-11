package gofilters

import (
	"fmt"
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

func NewTestFilter() engine.ToolFilter { return testFilter{} }

type testFilter struct{}

func (testFilter) Tool() string      { return "go test" }
func (testFilter) Aliases() []string { return nil }
func (testFilter) Prepare(args []string) engine.PrepareResult {
	if filtercommon.HasExactFlag(args, "-json") {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true, Ambiguous: true, Reason: "structured output mode"}
	}
	return engine.PrepareResult{NormalizedArgs: args}
}
func (testFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}
func (testFilter) MaskingHorizon() int { return 0 }

func (testFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream == engine.StderrStream {
		if ev.Type == engine.EventLine {
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		}
		return engine.Decision{Action: engine.ActionIgnore}
	}
	if ev.Type != engine.EventEOF && ev.Type != engine.EventExit {
		// Keep stdout in collect mode and emit one coherent compacted block at EOF/Exit.
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := mem.Joined()
	if strings.TrimSpace(raw) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	out, ok := compactTest(raw)
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}

func compactTest(raw string) (string, bool) {
	if strings.ContainsRune(raw, '\x00') {
		return "", false
	}
	lines := filtercommon.NonEmptyLines(raw)
	if len(lines) == 0 {
		return "go test: no output\n", true
	}
	state := collectGoTestCompactionState(lines)
	if state.recognized == 0 {
		return "", false
	}
	return renderGoTestCompaction(state), true
}

type goTestCompactionState struct {
	passed         map[string]struct{}
	skipped        map[string]struct{}
	failureDetails []string
	recognized     int
}

func collectGoTestCompactionState(lines []string) goTestCompactionState {
	state := goTestCompactionState{
		passed:         map[string]struct{}{},
		skipped:        map[string]struct{}{},
		failureDetails: make([]string, 0, maxDiagnostics),
	}
	for i := 0; i < len(lines); i++ {
		if nextIdx, handled := consumeGoTestCompactionLine(lines, i, &state); handled {
			i = nextIdx
		}
	}
	return state
}

func consumeGoTestCompactionLine(lines []string, idx int, state *goTestCompactionState) (int, bool) {
	line := lines[idx]
	if downloadingRe.MatchString(line) {
		return idx, true
	}
	if pkg, ok := parseGoTestOKPackage(line); ok {
		state.passed[pkg] = struct{}{}
		state.recognized++
		return idx, true
	}
	if pkg, ok := parseGoTestNoFilesPackage(line); ok {
		state.skipped[pkg] = struct{}{}
		state.recognized++
		return idx, true
	}
	if testFailPkgRe.MatchString(line) {
		state.recognized++
		return idx, true
	}
	if isGoTestFailureDetailStart(line) {
		return consumeGoTestFailureDetail(lines, idx, state), true
	}
	if buildIssueRe.MatchString(line) {
		state.recognized++
		return idx, true
	}
	return idx, false
}

func parseGoTestOKPackage(line string) (string, bool) {
	if m := testOKRe.FindStringSubmatch(line); len(m) > 1 {
		return m[1], true
	}
	return "", false
}

func parseGoTestNoFilesPackage(line string) (string, bool) {
	if m := testNoFilesRe.FindStringSubmatch(line); len(m) > 1 {
		return m[1], true
	}
	return "", false
}

func isGoTestFailureDetailStart(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "--- FAIL:") || strings.HasPrefix(trimmed, "panic:")
}

func consumeGoTestFailureDetail(lines []string, idx int, state *goTestCompactionState) int {
	state.failureDetails = append(state.failureDetails, lines[idx])
	state.recognized++
	if idx+1 >= len(lines) {
		return idx
	}
	nextRaw := lines[idx+1]
	next := strings.TrimSpace(nextRaw)
	if buildIssueRe.MatchString(next) || strings.HasPrefix(next, "\t") {
		state.failureDetails = append(state.failureDetails, nextRaw)
		return idx + 1
	}
	return idx
}

func renderGoTestCompaction(state goTestCompactionState) string {
	if len(state.failureDetails) > 0 {
		return renderGoTestFailureSummary(state)
	}
	return fmt.Sprintf("go test: %d passed, %d no-test-files\n", len(state.passed), len(state.skipped))
}

func renderGoTestFailureSummary(state goTestCompactionState) string {
	var b strings.Builder
	for i, line := range state.failureDetails {
		if i >= maxDiagnostics {
			_, _ = fmt.Fprintf(&b, "... +%d more\n", len(state.failureDetails)-maxDiagnostics)
			break
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	failed := countGoTestFailures(state.failureDetails)
	_, _ = fmt.Fprintf(&b, "go test: %d passed, %d failed, %d no-test-files\n", len(state.passed), failed, len(state.skipped))
	return b.String()
}

func countGoTestFailures(failureDetails []string) int {
	failed := 0
	for _, line := range failureDetails {
		if strings.HasPrefix(strings.TrimSpace(line), "--- FAIL:") {
			failed++
		}
	}
	if failed == 0 {
		return 1
	}
	return failed
}
