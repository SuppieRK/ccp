package filters

import (
	"regexp"
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

// gradleFilter implements semantic compaction for Gradle and Gradle Wrapper output.
type gradleFilter struct{}

// NewGradleFilter returns the built-in Gradle tool filter.
func NewGradleFilter() engine.ToolFilter { return gradleFilter{} }

func (gradleFilter) Tool() string { return "gradle" }

func (gradleFilter) Aliases() []string {
	return []string{"gradlew", "./gradlew", "gradlew.bat", "./gradlew.bat"}
}

func (gradleFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: append([]string{}, args...)}
}

func (gradleFilter) ContextKey(ev engine.Event) string {
	// Gradle frequently splits correlated failure context across stdout/stderr.
	// A shared key keeps task headers and failure diagnostics in one semantic view.
	return engine.SharedContextKey(ev)
}

func (gradleFilter) MaskingHorizon() int {
	// Gradle/JVM diagnostics can be deep into long lines.
	return 4096
}

func (gradleFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	// NOTE: Gradle diagnostics frequently split across stdout/stderr in shared context.
	// Keep collecting through EOF and flush only on Exit to avoid premature success flushes.
	if ev.Type != engine.EventExit {
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := mem.Joined()
	if strings.TrimSpace(raw) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	compacted, ok := compactGradleOutput(raw)
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	if strings.TrimSpace(compacted) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: compacted}
}

type gradleOutputClass int

const (
	gradleClassNeutral gradleOutputClass = iota
	gradleClassTaskBoundary
	gradleClassProgressNoise
	gradleClassFailureMarker
	gradleClassCausedBy
	gradleClassHelpPointer
	gradleClassStacktrace
	gradleClassSuccessSummary
)

var taskPrefixedLineRe = regexp.MustCompile(`^(:[A-Za-z0-9_.:-]+)\s+`)

func canonicalGradleLine(line string) string {
	return strings.TrimRight(strings.ReplaceAll(line, "\r", ""), "\n")
}

func classifyGradleLine(line string) (gradleOutputClass, string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return gradleClassNeutral, ""
	}

	if strings.HasPrefix(trimmed, "> Task ") {
		return gradleClassTaskBoundary, extractGradleTaskName(trimmed)
	}
	if isGradleProgressNoise(trimmed) {
		return gradleClassProgressNoise, ""
	}
	if strings.HasPrefix(trimmed, "Caused by:") {
		return gradleClassCausedBy, ""
	}
	if strings.Contains(trimmed, "https://help.gradle.org") {
		return gradleClassHelpPointer, ""
	}
	if isGradleFailureMarker(trimmed) {
		return gradleClassFailureMarker, ""
	}
	if strings.HasPrefix(trimmed, "at ") || strings.HasPrefix(trimmed, "\tat ") {
		return gradleClassStacktrace, ""
	}
	if strings.HasPrefix(trimmed, "BUILD SUCCESSFUL") || strings.Contains(trimmed, "actionable tasks:") {
		return gradleClassSuccessSummary, ""
	}
	return gradleClassNeutral, ""
}

func extractGradleTaskName(taskLine string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(taskLine, "> Task "))
	if rest == "" {
		return ""
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func isGradleProgressNoise(line string) bool {
	return strings.HasPrefix(line, "Downloading ") ||
		strings.HasPrefix(line, "Download ") ||
		(strings.Contains(line, "%") && strings.Contains(line, "[") && strings.Contains(line, "]")) ||
		strings.Contains(line, "KB/s") ||
		strings.Contains(line, "MB/s")
}

func isGradleFailureMarker(line string) bool {
	return strings.HasPrefix(line, "FAILURE: Build failed with an exception.") ||
		strings.HasPrefix(line, "BUILD FAILED") ||
		strings.Contains(line, "Execution failed for task") ||
		strings.HasPrefix(line, "* What went wrong:") ||
		strings.HasPrefix(line, "* Try:") ||
		strings.Contains(line, "Exception:") ||
		strings.HasPrefix(line, "Exception in thread")
}

func looksLikeTaskPrefixedLine(line string) (string, bool) {
	matches := taskPrefixedLineRe.FindStringSubmatch(strings.TrimSpace(line))
	if len(matches) != 2 {
		return "", false
	}
	return matches[1], true
}

type gradleTaskBlock struct {
	taskName string
	lines    []string
	failed   bool
}

func compactGradleOutput(raw string) (string, bool) {
	lines := filtercommon.NonEmptyLines(raw)
	state := newGradleCompactState()

	for _, rawLine := range lines {
		line := canonicalGradleLine(rawLine)
		if strings.TrimSpace(line) == "" {
			continue
		}
		state.consumeLine(line)
	}
	out, ok := state.finish()
	if !ok {
		return raw, false
	}
	if len(out) == 0 {
		return "", true
	}
	return strings.Join(out, "\n") + "\n", true
}

type gradleCompactState struct {
	out                     []string
	current                 *gradleTaskBlock
	retainCompilerContext   int
	lowConfidenceInterleave bool
}

func newGradleCompactState() *gradleCompactState {
	return &gradleCompactState{
		out: make([]string, 0, 16),
	}
}

func (s *gradleCompactState) consumeLine(line string) {
	s.trackInterleaving(line)
	class, task := classifyGradleLine(line)
	switch class {
	case gradleClassProgressNoise:
		return
	case gradleClassTaskBoundary:
		s.startTaskBoundary(line, task)
		return
	case gradleClassFailureMarker, gradleClassCausedBy, gradleClassHelpPointer, gradleClassStacktrace:
		s.appendFailureSignal(line)
		return
	case gradleClassSuccessSummary:
		s.flushCurrent()
		s.out = append(s.out, line)
		return
	default:
		s.appendDefaultLine(line)
		return
	}
}

func (s *gradleCompactState) trackInterleaving(line string) {
	if s.current == nil {
		return
	}
	prefixedTask, ok := looksLikeTaskPrefixedLine(line)
	if ok && prefixedTask != s.current.taskName {
		s.lowConfidenceInterleave = true
	}
}

func (s *gradleCompactState) startTaskBoundary(line, task string) {
	s.flushCurrent()
	s.current = &gradleTaskBlock{
		taskName: task,
		lines:    []string{line},
		// Gradle often reports explicit task failure at the boundary line.
		failed: strings.Contains(strings.ToUpper(line), " FAILED"),
	}
}

func (s *gradleCompactState) appendFailureSignal(line string) {
	if shouldSuppressFailureNoise(line) {
		return
	}
	if s.current != nil {
		s.current.failed = true
		s.current.lines = append(s.current.lines, line)
		return
	}
	s.out = append(s.out, line)
}

func (s *gradleCompactState) appendDefaultLine(line string) {
	trimmed := strings.TrimSpace(line)
	if strings.Contains(trimmed, ": error:") {
		s.retainCompilerContext = 3
		s.appendFailureSignal(line)
		return
	}
	if s.retainCompilerContext > 0 && looksCompilerContextLine(line) {
		s.retainCompilerContext--
		s.appendFailureSignal(line)
		return
	}
	if s.current != nil {
		s.appendCurrentTaskLine(line)
		return
	}
	if strings.HasPrefix(trimmed, "FAILURE:") ||
		strings.HasPrefix(trimmed, "BUILD ") ||
		strings.HasPrefix(trimmed, "* What went wrong:") ||
		strings.HasPrefix(trimmed, "* Try:") ||
		strings.Contains(trimmed, "help.gradle.org") {
		s.out = append(s.out, line)
	}
}

func (s *gradleCompactState) appendCurrentTaskLine(line string) {
	// Preserve concrete failure locations even if failure marker appears later.
	trimmed := strings.TrimSpace(line)
	if !s.current.failed && (strings.Contains(trimmed, ": error:") || strings.Contains(trimmed, "Compilation failed")) {
		s.current.failed = true
	}
	if !s.current.failed || shouldSuppressFailureNoise(line) {
		return
	}
	s.current.lines = append(s.current.lines, line)
}

func (s *gradleCompactState) flushCurrent() {
	if s.current == nil {
		return
	}
	if s.current.failed {
		s.out = append(s.out, s.current.lines...)
	} else if s.current.taskName != "" {
		s.out = append(s.out, "[ok] "+s.current.taskName)
	}
	s.current = nil
}

func (s *gradleCompactState) finish() ([]string, bool) {
	s.flushCurrent()
	if s.lowConfidenceInterleave {
		return nil, false
	}
	return s.out, true
}

func looksCompilerContextLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "^") {
		return true
	}
	if strings.Contains(trimmed, " error") {
		return true
	}
	if strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "return ") {
		return true
	}
	// preserve indented source snippets that typically follow compiler diagnostics
	return strings.HasPrefix(line, " ")
}

func shouldSuppressFailureNoise(line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "at org.gradle.") || strings.HasPrefix(trimmed, "\tat org.gradle.") {
		return true
	}
	if strings.HasPrefix(trimmed, "at java.base") || strings.HasPrefix(trimmed, "\tat java.base") {
		return true
	}
	// Runtime shutdown logs are usually post-failure noise and add little diagnostic value.
	if strings.Contains(trimmed, "ShutdownHook") && (strings.Contains(trimmed, "Shutdown initiated") || strings.Contains(trimmed, "Shutdown completed")) {
		return true
	}
	return false
}
