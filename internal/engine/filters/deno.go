package filters

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

var denoLifecyclePrefixRe = regexp.MustCompile(`^(Download|Check|Compile|Bundle)\s+(http|https|file)://`)
var denoPromptLineRe = regexp.MustCompile(`\?\s+\[[^\]]+\]\s*$`)
var denoTSErrorLineRe = regexp.MustCompile(`^(TS\d+)\s+\[ERROR\]:\s*(.+)$`)

const (
	denoOKMarker   = "ok\n"
	denoErrorLabel = "error:"
)

// NewDenoFilter returns the built-in deno tool filter.
func NewDenoFilter() engine.ToolFilter { return denoFilter{} }

type denoFilter struct{}

func (denoFilter) Tool() string { return "deno" }

func (denoFilter) Aliases() []string {
	return []string{"deno.exe", "./deno.exe", "deno.cmd", "./deno.cmd"}
}

func (denoFilter) Prepare(args []string) engine.PrepareResult {
	if len(args) == 0 {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}

	normalized := append([]string{}, args...)
	if hasStructuredDenoOutput(args) {
		return engine.PrepareResult{
			NormalizedArgs:   normalized,
			DispatchKey:      "deno|mode=structured",
			ForcePassthrough: true,
			Ambiguous:        true,
			Reason:           "structured output mode",
		}
	}

	mode := denoModeFromArgs(args)
	if mode == "repl" || mode == "task" || mode == "unknown" {
		return engine.PrepareResult{
			NormalizedArgs:   normalized,
			DispatchKey:      "deno|mode=" + mode,
			ForcePassthrough: true,
		}
	}
	return engine.PrepareResult{
		NormalizedArgs: normalized,
		DispatchKey:    "deno|mode=" + mode,
	}
}

func (denoFilter) ContextKey(ev engine.Event) string {
	// Deno diagnostics can span stdout+stderr; keep one shared context.
	return engine.SharedContextKey(ev)
}

func (denoFilter) MaskingHorizon() int { return 4096 }

func (denoFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	cfg := parseDenoDispatch(ev.Dispatch)
	switch filtercommon.LowerTrim(cfg.mode) {
	case "structured", "repl", "task", "unknown":
		switch ev.Type {
		case engine.EventLine:
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		case engine.EventTick, engine.EventEOF, engine.EventExit:
			return engine.Decision{Action: engine.ActionIgnore}
		default:
			return engine.Decision{Action: engine.ActionCollect}
		}
	}

	if d, ok := processImmediateDenoSignal(ev, mem, cfg); ok {
		return d
	}

	switch ev.Type {
	case engine.EventTick, engine.EventLine, engine.EventEOF:
		return engine.Decision{Action: engine.ActionCollect}
	case engine.EventExit:
		return processDenoExit(mem.Joined(), cfg, ev.ExitCode)
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

func processImmediateDenoSignal(ev engine.Event, mem *engine.OrderedSetBuffer, cfg denoDispatch) (engine.Decision, bool) {
	if ev.Type != engine.EventLine || !isImmediateDenoSignal(ev.Line) {
		return engine.Decision{}, false
	}
	if mem.Len() <= 1 {
		return engine.Decision{Action: engine.ActionFlush, Output: ev.Line}, true
	}
	raw := mem.Joined()
	out, ok := compactDenoOutput(raw, cfg, 1)
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}, true
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}, true
}

func processDenoExit(raw string, cfg denoDispatch, exitCode int) engine.Decision {
	if strings.TrimSpace(raw) == "" {
		if exitCode == 0 {
			return engine.Decision{Action: engine.ActionFlush, Output: denoOKMarker}
		}
		return engine.Decision{Action: engine.ActionIgnore}
	}
	out, ok := compactDenoOutput(raw, cfg, exitCode)
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	if strings.TrimSpace(out) == "" {
		if exitCode == 0 {
			return engine.Decision{Action: engine.ActionFlush, Output: denoOKMarker}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}

type denoDispatch struct {
	mode string
}

func parseDenoDispatch(dispatch string) denoDispatch {
	cfg := denoDispatch{mode: "run"}
	if mode := strings.TrimSpace(filtercommon.DispatchValue(dispatch, "mode")); mode != "" {
		cfg.mode = mode
	}
	return cfg
}

func denoModeFromArgs(args []string) string {
	for _, arg := range args {
		a := filtercommon.LowerTrim(arg)
		if a == "" {
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		switch a {
		case "run", "test", "lint", "check":
			return a
		case "fmt":
			return "lint"
		case "repl", "task":
			return a
		default:
			return "unknown"
		}
	}
	return "unknown"
}

func hasStructuredDenoOutput(args []string) bool {
	return filtercommon.HasExactFlag(args, "--json") || filtercommon.HasOption(args, "--json")
}

func compactDenoOutput(raw string, cfg denoDispatch, exitCode int) (string, bool) {
	if lowConfidenceDenoOutput(raw) {
		return raw, false
	}

	lines := filtercommon.NonEmptyLines(raw)
	out, order, typeState, failureDetected := collectDenoOutputLines(lines)

	// Add compacted progress retain-first summaries before payloads only for non-failure flows.
	out = appendDenoProgressPrefix(out, order, typeState, failureDetected)
	return finalizeCompactDenoOutput(out, cfg, exitCode, failureDetected), true
}

func appendDenoProgressPrefix(out []string, order []string, typeState map[string]*denoProgressFold, failureDetected bool) []string {
	if len(order) == 0 || failureDetected {
		return out
	}
	prefix := make([]string, 0, len(order)*2)
	for _, key := range order {
		st := typeState[key]
		prefix = append(prefix, st.first)
		if st.count > 1 {
			prefix = append(prefix, fmt.Sprintf("[+%d similar progress lines]", st.count-1))
		}
	}
	return append(prefix, out...)
}

func finalizeCompactDenoOutput(out []string, cfg denoDispatch, exitCode int, failureDetected bool) string {
	if len(out) == 0 {
		if exitCode == 0 {
			return denoOKMarker
		}
		return ""
	}
	if cfg.mode == "test" && failureDetected {
		return strings.Join(compactDenoFailureLines(out), "\n") + "\n"
	}
	if exitCode == 0 && onlyProgressOrBoilerplate(out) {
		return denoOKMarker
	}
	return strings.Join(out, "\n") + "\n"
}

func collectDenoOutputLines(lines []string) ([]string, []string, map[string]*denoProgressFold, bool) {
	out := make([]string, 0, len(lines))
	seenPayload := map[string]struct{}{}
	typeState := map[string]*denoProgressFold{}
	order := make([]string, 0, len(lines))
	failureDetected := false
	for _, rawLine := range lines {
		line := strings.TrimRight(strings.ReplaceAll(rawLine, "\r", ""), "\n")
		class := classifyDenoLine(rawLine, line)
		switch class {
		case denoClassProgress:
			key := denoProgressFoldKey(line)
			st := typeState[key]
			if st == nil {
				st = &denoProgressFold{first: line}
				typeState[key] = st
				order = append(order, key)
			}
			st.count++
			continue
		case denoClassFailure:
			failureDetected = true
		case denoClassPayload:
			// continue to payload retention below
		default:
			// Unknown class: preserve as payload for safety.
		}
		norm := strings.TrimSpace(line)
		if _, ok := seenPayload[norm]; ok {
			continue
		}
		seenPayload[norm] = struct{}{}
		out = append(out, line)
	}
	return out, order, typeState, failureDetected
}

func compactDenoFailureLines(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		nextIndex, extracted := compactSingleDenoFailureLine(lines, i)
		i = nextIndex
		out = append(out, extracted...)
	}
	if len(out) == 0 {
		return lines
	}
	return out
}

func compactSingleDenoFailureLine(lines []string, index int) (int, []string) {
	line := strings.TrimSpace(lines[index])
	if line == "" || denoLifecyclePrefixRe.MatchString(line) {
		return index, nil
	}
	if denoLine, nextIdx, ok := parseDenoTSError(lines, index, line); ok {
		return nextIdx, []string{denoLine}
	}
	lower := strings.ToLower(line)
	if !strings.HasPrefix(lower, denoErrorLabel) &&
		!strings.HasPrefix(lower, "hint:") &&
		!strings.Contains(lower, "panic:") &&
		!strings.HasPrefix(lower, "stack backtrace:") {
		return index, nil
	}
	out := []string{line}
	if strings.HasPrefix(lower, denoErrorLabel) && index+1 < len(lines) {
		next := strings.TrimSpace(lines[index+1])
		if strings.HasPrefix(next, "at ") {
			out = append(out, next)
			return index + 1, out
		}
	}
	return index, out
}

func parseDenoTSError(lines []string, start int, line string) (string, int, bool) {
	m := denoTSErrorLineRe.FindStringSubmatch(line)
	if len(m) == 0 {
		return "", start, false
	}
	code := m[1]
	msg := m[2]
	loc := ""
	lastIdx := start
	for j := start + 1; j < len(lines); j++ {
		next := strings.TrimSpace(lines[j])
		if strings.HasPrefix(next, "at ") {
			loc = strings.TrimPrefix(next, "at ")
			lastIdx = j
			break
		}
		if denoTSErrorLineRe.MatchString(next) || strings.HasPrefix(strings.ToLower(next), denoErrorLabel) {
			break
		}
	}
	if loc != "" {
		return code + " [ERROR] at " + loc + ": " + msg, lastIdx, true
	}
	return code + " [ERROR]: " + msg, lastIdx, true
}

type denoProgressFold struct {
	first string
	count int
}

type denoOutputClass int

const (
	denoClassPayload denoOutputClass = iota
	denoClassProgress
	denoClassFailure
)

func classifyDenoLine(rawLine, line string) denoOutputClass {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return denoClassProgress
	}
	if isImmediateDenoSignal(trimmed) {
		return denoClassFailure
	}
	lower := filtercommon.LowerTrim(trimmed)
	if strings.Contains(lower, "error") || strings.Contains(lower, "failed") {
		return denoClassFailure
	}
	if denoLifecyclePrefixRe.MatchString(trimmed) {
		return denoClassProgress
	}
	if strings.Contains(rawLine, "\r") || strings.Contains(trimmed, "\x1b[") {
		return denoClassProgress
	}
	if strings.ContainsAny(trimmed, "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏") {
		return denoClassProgress
	}
	return denoClassPayload
}

func isImmediateDenoSignal(line string) bool {
	trimmed := strings.TrimSpace(line)
	lower := filtercommon.LowerTrim(trimmed)
	if strings.Contains(lower, "panic:") || strings.Contains(lower, "stack backtrace:") {
		return true
	}
	return denoPromptLineRe.MatchString(trimmed)
}

func denoProgressFoldKey(line string) string {
	if !denoLifecyclePrefixRe.MatchString(line) {
		return filtercommon.LowerTrim(line)
	}
	parts := strings.SplitN(line, " ", 2)
	return filtercommon.LowerTrim(parts[0])
}

func lowConfidenceDenoOutput(raw string) bool {
	if strings.ContainsRune(raw, '\x00') {
		return true
	}
	total := 0
	control := 0
	for _, r := range raw {
		if !utf8.ValidRune(r) {
			return true
		}
		total++
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			control++
		}
	}
	if total == 0 {
		return false
	}
	return control*100/total > 20
}

func onlyProgressOrBoilerplate(lines []string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, "[+") && strings.HasSuffix(line, "similar progress lines]") {
			continue
		}
		if denoLifecyclePrefixRe.MatchString(strings.TrimSpace(line)) {
			continue
		}
		return false
	}
	return len(lines) > 0
}
