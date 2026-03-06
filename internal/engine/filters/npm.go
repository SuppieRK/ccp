package filters

import (
	"regexp"
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

const npmOKMarker = "ok\n"

// NewNPMFilter returns the built-in npm tool filter.
func NewNPMFilter() engine.ToolFilter { return npmFilter{} }

type npmFilter struct{}

func (npmFilter) Tool() string { return "npm" }

func (npmFilter) Aliases() []string {
	return []string{"npm.cmd", "./npm.cmd", "npm.exe", "./npm.exe"}
}

func (npmFilter) Prepare(args []string) engine.PrepareResult {
	passthrough := engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	if len(args) == 0 {
		return passthrough
	}
	if filtercommon.LowerTrim(args[0]) != "run" {
		return passthrough
	}
	return engine.PrepareResult{NormalizedArgs: append([]string{}, args...), DispatchKey: "npm|mode=run"}
}

func (npmFilter) ContextKey(ev engine.Event) string {
	// npm failures often mix stdout/stderr; keep one shared semantic context.
	return engine.SharedContextKey(ev)
}

func (npmFilter) MaskingHorizon() int { return 4096 }

func (npmFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Type != engine.EventExit {
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := mem.Joined()
	if strings.TrimSpace(raw) == "" {
		if ev.ExitCode == 0 {
			return engine.Decision{Action: engine.ActionFlush, Output: npmOKMarker}
		}
		return engine.Decision{Action: engine.ActionIgnore}
	}
	out, ok := compactNPMOutput(raw, parseNPMDispatch(ev.Dispatch), ev.ExitCode)
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	if strings.TrimSpace(out) != "" {
		return engine.Decision{Action: engine.ActionFlush, Output: out}
	}
	if ev.ExitCode == 0 {
		return engine.Decision{Action: engine.ActionFlush, Output: npmOKMarker}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: raw}
}

type npmDispatch struct {
	mode string
}

func parseNPMDispatch(dispatch string) npmDispatch {
	if mode := strings.TrimSpace(filtercommon.DispatchValue(dispatch, "mode")); mode != "" {
		return npmDispatch{mode: mode}
	}
	return npmDispatch{mode: "run"}
}

type npmOutputClass int

const (
	npmClassNeutral npmOutputClass = iota
	npmClassLifecycle
	npmClassProgress
	npmClassWarning
	npmClassFailure
)

var npmLifecycleHeaderRe = regexp.MustCompile(`^>\s+[^\s]+@[^\s]+\s+[^\s]+`)

func classifyNPMLine(rawLine, line string) npmOutputClass {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return npmClassProgress
	}
	lower := strings.ToLower(trimmed)
	leftLower := strings.ToLower(strings.TrimLeft(trimmed, " \t"))

	if isNPMLifecycleLine(trimmed, lower) {
		return npmClassLifecycle
	}
	if isNPMProgressLine(leftLower, lower) || isNPMProgressNoise(rawLine, trimmed) {
		return npmClassProgress
	}
	if isNPMWarningLine(leftLower, lower) {
		return npmClassWarning
	}
	if isNPMFailureLine(trimmed, leftLower, lower) {
		return npmClassFailure
	}
	return npmClassNeutral
}

func isNPMLifecycleLine(trimmed, lower string) bool {
	return npmLifecycleHeaderRe.MatchString(trimmed) ||
		strings.HasPrefix(trimmed, "> ") ||
		strings.HasPrefix(lower, "yarn run v") ||
		strings.HasPrefix(trimmed, "$ ")
}

func isNPMProgressLine(leftLower, lower string) bool {
	return strings.HasPrefix(leftLower, "npm notice") ||
		strings.HasPrefix(lower, "done in ") ||
		strings.HasPrefix(lower, "info visit https://yarnpkg.com")
}

func isNPMWarningLine(leftLower, lower string) bool {
	return strings.HasPrefix(leftLower, "npm warn") || strings.HasPrefix(lower, "warning ")
}

func isNPMFailureLine(trimmed, leftLower, lower string) bool {
	return strings.HasPrefix(leftLower, "npm err!") ||
		strings.HasPrefix(lower, "error command failed with exit code") ||
		strings.Contains(lower, " failed") ||
		strings.Contains(lower, " error") ||
		strings.HasPrefix(trimmed, "[ERROR]")
}

func isNPMProgressNoise(rawLine, trimmed string) bool {
	return strings.Contains(rawLine, "\r") ||
		strings.Contains(trimmed, "\x1b[") ||
		strings.ContainsAny(trimmed, "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏") ||
		strings.Trim(trimmed, ".") == ""
}

func compactNPMOutput(raw string, cfg npmDispatch, exitCode int) (string, bool) {
	if cfg.mode != "run" {
		return raw, false
	}
	if lowConfidenceNPMOutput(raw) {
		return raw, false
	}

	lines := filtercommon.NonEmptyLines(raw)
	out := make([]string, 0, len(lines))
	seen := map[string]struct{}{}
	seenFooter := map[string]struct{}{}

	for _, rawLine := range lines {
		line := strings.TrimRight(strings.ReplaceAll(rawLine, "\r", ""), "\n")
		if strings.TrimSpace(line) == "" {
			continue
		}
		class := classifyNPMLine(rawLine, line)
		next, keep := shouldAppendNPMLine(class, line, seen, seenFooter)
		if !keep {
			continue
		}
		out = append(out, next)
	}

	if len(out) == 0 {
		if exitCode == 0 {
			return npmOKMarker, true
		}
		return "", true
	}
	return strings.Join(out, "\n") + "\n", true
}

func shouldAppendNPMLine(class npmOutputClass, line string, seen, seenFooter map[string]struct{}) (string, bool) {
	switch class {
	case npmClassLifecycle, npmClassProgress:
		return "", false
	case npmClassFailure, npmClassWarning, npmClassNeutral:
		// handled below
	default:
		return "", false
	}
	if class == npmClassFailure {
		lower := filtercommon.LowerTrim(line)
		if strings.Contains(lower, "a complete log of this run can be found in") ||
			(strings.Contains(lower, "debug") && strings.Contains(lower, "log") && strings.Contains(lower, "npm")) {
			if _, ok := seenFooter[line]; ok {
				return "", false
			}
			seenFooter[line] = struct{}{}
		}
	}
	k := strings.TrimSpace(line)
	if _, ok := seen[k]; ok {
		return "", false
	}
	seen[k] = struct{}{}
	return line, true
}

func lowConfidenceNPMOutput(raw string) bool {
	if strings.ContainsRune(raw, '\x00') {
		return true
	}
	total := 0
	control := 0
	for _, r := range raw {
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
