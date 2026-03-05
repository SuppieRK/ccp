package common

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var nodePIDPrefixRe = regexp.MustCompile(`^\(node:\d+\)\s+`)

func NodeNormalizeWarningKey(line string) string {
	return strings.ToLower(nodePIDPrefixRe.ReplaceAllString(strings.TrimSpace(line), "(node:PID) "))
}

func NodeIsRuntimeWarning(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	return nodePIDPrefixRe.MatchString(line) ||
		strings.Contains(lower, "experimentalwarning") ||
		strings.Contains(lower, "deprecationwarning") ||
		strings.Contains(lower, "to load an es module") ||
		strings.Contains(lower, `set "type": "module"`) ||
		strings.Contains(lower, "require() of es module")
}

func NodeIsProgressNoise(rawLine, trimmed string) bool {
	return strings.Contains(rawLine, "\r") ||
		strings.Contains(trimmed, "\x1b[") ||
		strings.ContainsAny(trimmed, "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
}

func NodeIsUnhandledFailure(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "unhandledpromiserejection") ||
		strings.Contains(lower, "unhandled rejection") ||
		strings.Contains(lower, "uncaught exception")
}

func NodeCanonicalLine(line string) string {
	return strings.TrimRight(strings.ReplaceAll(line, "\r", ""), "\n")
}

func NodeIsInteractiveInvocation(args []string) bool {
	if len(args) == 0 {
		return true
	}
	inlineEval := false
	expectValue := false
	for i := 0; i < len(args); i++ {
		a := strings.TrimSpace(args[i])
		if a == "" {
			continue
		}
		if expectValue {
			expectValue = false
			continue
		}
		switch a {
		case "-i", "--interactive":
			return true
		case "-e", "--eval", "-p", "--print":
			inlineEval = true
			expectValue = true
			continue
		case "--":
			return i == len(args)-1
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		return false
	}
	return !inlineEval
}

func NodeLowConfidenceOutput(raw string) bool {
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
