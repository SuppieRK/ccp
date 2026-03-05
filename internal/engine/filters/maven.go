package filters

import (
	"fmt"
	"regexp"
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

// NewMavenFilter returns the built-in Maven/Maven Wrapper tool filter.
func NewMavenFilter() engine.ToolFilter { return mavenFilter{} }

type mavenFilter struct{}

func (mavenFilter) Tool() string { return "maven" }

func (mavenFilter) Aliases() []string {
	return []string{"mvnw", "./mvnw", "mvnw.cmd", "./mvnw.cmd", "mvn.cmd", "./mvn.cmd", "mvn.bat", "./mvn.bat"}
}

func (mavenFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{
		NormalizedArgs: append([]string{}, args...),
		DispatchKey:    "maven|parallel=" + boolAs01(hasMavenParallelFlag(args)),
	}
}

func (mavenFilter) ContextKey(ev engine.Event) string {
	// Maven failures are frequently split across stdout/stderr; keep shared context.
	return engine.SharedContextKey(ev)
}

func (mavenFilter) MaskingHorizon() int { return 4096 }

func (mavenFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	// NOTE: Maven failures are frequently split across stdout/stderr.
	// Do not flush on EOF because one stream can EOF earlier and incorrectly
	// emit a success summary before failure diagnostics arrive on the other stream.
	// Flush only on Exit so compaction sees the full shared context.
	if ev.Type != engine.EventExit {
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := mem.Joined()
	if strings.TrimSpace(raw) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	out, ok := compactMavenOutput(raw, parseMavenDispatch(ev.Dispatch))
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	if strings.TrimSpace(out) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}

type mavenDispatch struct {
	parallel bool
}

func parseMavenDispatch(dispatch string) mavenDispatch {
	return mavenDispatch{parallel: filtercommon.ParseBool01(filtercommon.DispatchValue(dispatch, "parallel"))}
}

func hasMavenParallelFlag(args []string) bool {
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "-T" || arg == "--threads" || strings.HasPrefix(arg, "--threads=") {
			return true
		}
		if len(arg) > 2 && strings.HasPrefix(arg, "-T") {
			return true
		}
	}
	return false
}

type mavenOutputClass int

const (
	mavenClassNeutral mavenOutputClass = iota
	mavenClassScopeBoundary
	mavenClassProgressNoise
	mavenClassTransferFailure
	mavenClassFailureMarker
	mavenClassCausedBy
	mavenClassStacktrace
	mavenClassReactorSummary
	mavenClassFinalStatus
)

type mavenScope struct {
	module string
	goal   string
	failed bool
	lines  []string
	seen   map[string]struct{}
}

type mavenClassMeta struct {
	module string
	goal   string
}

type mavenCompactState struct {
	cfg              mavenDispatch
	out              []string
	transferWindow   []string
	threadIDs        map[string]struct{}
	inReactorSummary bool
	sawScopeBoundary bool
	buildFailed      bool
	current          *mavenScope
}

var (
	mavenGoalBoundaryRe = regexp.MustCompile(`^\[INFO\]\s+---\s+.+:([A-Za-z0-9_-]+)\s+\([^)]+\)\s+@\s+([^ ]+)\s+---$`)
	mavenThreadPrefixRe = regexp.MustCompile(`^\[(Thread-\d+|\d+)\]\s+`)
)

func classifyMavenLine(line string) (mavenOutputClass, mavenClassMeta) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return mavenClassNeutral, mavenClassMeta{}
	}
	if strings.HasPrefix(trimmed, "[INFO] Reactor Build Order") || strings.HasPrefix(trimmed, "[INFO] Reactor Summary") {
		return mavenClassReactorSummary, mavenClassMeta{}
	}
	if strings.Contains(trimmed, "BUILD SUCCESS") || strings.Contains(trimmed, "BUILD FAILURE") {
		return mavenClassFinalStatus, mavenClassMeta{}
	}
	if goal, module, ok := parseMavenGoalBoundary(trimmed); ok {
		return mavenClassScopeBoundary, mavenClassMeta{module: module, goal: goal}
	}
	if isMavenProgressNoise(trimmed) {
		return mavenClassProgressNoise, mavenClassMeta{}
	}
	if isMavenTransferFailure(trimmed) {
		return mavenClassTransferFailure, mavenClassMeta{}
	}
	if strings.HasPrefix(trimmed, "Caused by:") {
		return mavenClassCausedBy, mavenClassMeta{}
	}
	if strings.HasPrefix(trimmed, "[ERROR]") || strings.Contains(trimmed, "Failed to execute goal") {
		return mavenClassFailureMarker, mavenClassMeta{}
	}
	if isMavenStackLine(trimmed) {
		return mavenClassStacktrace, mavenClassMeta{}
	}
	return mavenClassNeutral, mavenClassMeta{}
}

func parseMavenGoalBoundary(line string) (goal string, module string, ok bool) {
	m := mavenGoalBoundaryRe.FindStringSubmatch(line)
	if len(m) != 3 {
		return "", "", false
	}
	return strings.TrimSpace(m[1]), strings.TrimSpace(m[2]), true
}

func isMavenProgressNoise(line string) bool {
	return strings.Contains(line, "Downloading from ") ||
		strings.Contains(line, "Downloaded from ") ||
		strings.Contains(line, "Progress (") ||
		strings.Contains(line, "B at ")
}

func isMavenTransferFailure(line string) bool {
	return strings.Contains(line, "Could not transfer artifact") ||
		strings.Contains(line, "transfer failed for") ||
		strings.Contains(line, "Failed to read artifact descriptor")
}

func isMavenStackLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "at ")
}

func compactMavenOutput(raw string, cfg mavenDispatch) (string, bool) {
	lines := filtercommon.NonEmptyLines(raw)
	state := mavenCompactState{
		cfg:            cfg,
		out:            make([]string, 0, len(lines)),
		transferWindow: make([]string, 0, 3),
		threadIDs:      map[string]struct{}{},
	}

	for _, rawLine := range lines {
		line := strings.TrimRight(strings.ReplaceAll(rawLine, "\r", ""), "\n")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if state.consumeLine(line) {
			return raw, false
		}
	}

	state.flushScope()
	if len(state.out) == 0 {
		return "", true
	}
	return strings.Join(state.out, "\n") + "\n", true
}

func (s *mavenCompactState) consumeLine(line string) (passthrough bool) {
	threadID, stripped := stripMavenThreadPrefix(line)
	if threadID != "" {
		s.threadIDs[threadID] = struct{}{}
		line = stripped
	}
	class, meta := classifyMavenLine(line)
	if class == mavenClassScopeBoundary {
		s.sawScopeBoundary = true
	}
	if s.cfg.parallel && len(s.threadIDs) > 1 && !s.sawScopeBoundary {
		return true
	}
	s.handleClass(line, class, meta)
	return false
}

func (s *mavenCompactState) appendCurrentLine(line string) {
	if s.current == nil {
		s.current = &mavenScope{seen: map[string]struct{}{}}
	}
	if s.current.seen == nil {
		s.current.seen = map[string]struct{}{}
	}
	_, k := stripMavenThreadPrefix(strings.TrimSpace(line))
	if _, exists := s.current.seen[k]; exists {
		return
	}
	s.current.seen[k] = struct{}{}
	s.current.lines = append(s.current.lines, line)
}

func (s *mavenCompactState) flushScope() {
	if s.current == nil {
		return
	}
	if s.buildFailed {
		s.current.failed = true
	}
	if s.current.failed {
		if s.current.module != "" && s.current.goal != "" {
			s.out = append(s.out, fmt.Sprintf("[x] %s : %s", s.current.module, s.current.goal))
		}
		s.out = append(s.out, s.current.lines...)
	} else if s.current.module != "" && s.current.goal != "" {
		s.out = append(s.out, fmt.Sprintf("[ok] %s : %s", s.current.module, s.current.goal))
	}
	s.current = nil
}

func (s *mavenCompactState) handleClass(line string, class mavenOutputClass, meta mavenClassMeta) {
	switch class {
	case mavenClassProgressNoise:
		s.recordTransferLine(line)
	case mavenClassScopeBoundary:
		s.startScope(meta)
	case mavenClassTransferFailure:
		s.appendFailureWithTransfers(line)
	case mavenClassFailureMarker, mavenClassCausedBy, mavenClassStacktrace:
		s.appendFailureLine(line)
	case mavenClassReactorSummary:
		s.beginReactorSummary(line)
	case mavenClassFinalStatus:
		s.completeBuild(line)
	default:
		s.handleDefaultLine(line)
	}
}

func (s *mavenCompactState) recordTransferLine(line string) {
	if len(s.transferWindow) == cap(s.transferWindow) {
		s.transferWindow = s.transferWindow[1:]
	}
	s.transferWindow = append(s.transferWindow, line)
}

func (s *mavenCompactState) startScope(meta mavenClassMeta) {
	s.flushScope()
	s.current = &mavenScope{module: meta.module, goal: meta.goal, seen: map[string]struct{}{}}
	s.transferWindow = s.transferWindow[:0]
}

func (s *mavenCompactState) ensureScope() {
	if s.current == nil {
		s.current = &mavenScope{seen: map[string]struct{}{}}
	}
}

func (s *mavenCompactState) appendFailureLine(line string) {
	s.ensureScope()
	s.current.failed = true
	s.appendCurrentLine(line)
}

func (s *mavenCompactState) appendFailureWithTransfers(line string) {
	s.ensureScope()
	s.current.failed = true
	for _, transfer := range s.transferWindow {
		s.appendCurrentLine(transfer)
	}
	s.transferWindow = s.transferWindow[:0]
	s.appendCurrentLine(line)
}

func (s *mavenCompactState) beginReactorSummary(line string) {
	s.flushScope()
	s.inReactorSummary = true
	s.out = append(s.out, line)
}

func (s *mavenCompactState) completeBuild(line string) {
	if strings.Contains(line, "BUILD FAILURE") {
		s.buildFailed = true
		if s.current != nil {
			s.current.failed = true
		}
	}
	s.flushScope()
	s.inReactorSummary = false
	s.out = append(s.out, line)
}

func (s *mavenCompactState) handleDefaultLine(line string) {
	if s.inReactorSummary && strings.HasPrefix(strings.TrimSpace(line), "[INFO]") {
		s.out = append(s.out, line)
		return
	}
	if s.current != nil && s.current.failed {
		s.appendCurrentLine(line)
	}
}

func stripMavenThreadPrefix(line string) (threadID string, stripped string) {
	m := mavenThreadPrefixRe.FindStringSubmatch(line)
	if len(m) != 2 {
		return "", line
	}
	return m[1], strings.TrimSpace(strings.TrimPrefix(line, m[0]))
}
