package prettiercommon

import (
	"fmt"
	"regexp"
	"strings"
)

var pathOnlyRe = regexp.MustCompile(`^[^\s].*\.(js|jsx|ts|tsx|mjs|cjs|json|md|css|scss|yaml|yml|html|graphql)$`)
var pathWithTimingRe = regexp.MustCompile(`^([^\s].*\.(js|jsx|ts|tsx|mjs|cjs|json|md|css|scss|yaml|yml|html|graphql))\s+\d+(\.\d+)?ms$`)

func SummarizeOutput(raw string) (string, bool) {
	state := summaryState{
		needsFormatting: make([]string, 0),
		formatted:       make([]string, 0),
		other:           make([]string, 0),
	}
	for _, line := range strings.Split(raw, "\n") {
		consumeSummaryLine(&state, line)
	}
	return renderSummary(state)
}

func consumeSummaryLine(state *summaryState, line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}
	lower := strings.ToLower(trimmed)

	switch {
	case strings.HasPrefix(lower, "checking formatting"):
		state.hasCheckStart = true
	case strings.Contains(lower, "all matched files use prettier code style"):
		state.hasCheckSuccess = true
	case strings.Contains(lower, "code style issues found in"):
		state.hasCheckFailure = true
	case strings.HasPrefix(lower, "[warn] "):
		consumeWarnLine(state, trimmed)
	case strings.HasPrefix(lower, "[error] "):
		state.other = append(state.other, trimmed)
	case pathOnlyRe.MatchString(trimmed):
		state.needsFormatting = appendUnique(state.needsFormatting, trimmed)
	default:
		consumeOtherLine(state, trimmed)
	}
}

func consumeWarnLine(state *summaryState, trimmed string) {
	path := strings.TrimSpace(trimmed[len("[warn] "):])
	if pathOnlyRe.MatchString(path) {
		state.needsFormatting = appendUnique(state.needsFormatting, path)
		return
	}
	state.other = append(state.other, trimmed)
}

func consumeOtherLine(state *summaryState, trimmed string) {
	if m := pathWithTimingRe.FindStringSubmatch(trimmed); len(m) > 1 {
		state.formatted = appendUnique(state.formatted, m[1])
		return
	}
	state.other = append(state.other, trimmed)
}

func renderSummary(state summaryState) (string, bool) {
	if len(state.needsFormatting) > 0 && state.hasCheckFailure {
		return renderCheckFailureSummary(state.needsFormatting), true
	}
	if state.hasCheckStart && state.hasCheckSuccess && len(state.needsFormatting) == 0 && len(state.other) == 0 {
		return "prettier check: ok\n", true
	}
	if len(state.formatted) > 1 && len(state.other) == 0 {
		return renderWriteSummary(state.formatted), true
	}
	return "", false
}

func renderCheckFailureSummary(paths []string) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "prettier check: %d files need formatting\n", len(paths))
	for _, path := range paths {
		b.WriteString("- ")
		b.WriteString(path)
		b.WriteString("\n")
	}
	return b.String()
}

func renderWriteSummary(paths []string) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "prettier write: formatted %d files\n", len(paths))
	for _, path := range paths {
		b.WriteString("- ")
		b.WriteString(path)
		b.WriteString("\n")
	}
	return b.String()
}

func appendUnique(items []string, v string) []string {
	for _, item := range items {
		if item == v {
			return items
		}
	}
	return append(items, v)
}

type summaryState struct {
	needsFormatting []string
	formatted       []string
	other           []string
	hasCheckStart   bool
	hasCheckSuccess bool
	hasCheckFailure bool
}
