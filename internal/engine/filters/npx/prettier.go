package npxfilters

import (
	"fmt"
	"regexp"
	"strings"

	"go-command-compression-proxy/internal/engine"
)

func NewNpxPrettierFilter() engine.ToolFilter { return prettierFilter{} }

type prettierFilter struct{}

func (prettierFilter) Tool() string      { return "npx prettier" }
func (prettierFilter) Aliases() []string { return nil }
func (prettierFilter) MaskingHorizon() int {
	return 0
}
func (prettierFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: append([]string{}, args...)}
}
func (prettierFilter) ContextKey(ev engine.Event) string {
	return engine.SharedContextKeyForTool(ev.CommandID, ev.Tool)
}
func (prettierFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
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
		withoutWrapper := stripNpxWrapperNoise(raw)
		if strings.TrimSpace(withoutWrapper) == "" {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		if summary, ok := summarizePrettierOutput(withoutWrapper); ok {
			return engine.Decision{Action: engine.ActionFlush, Output: summary}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: withoutWrapper}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

var prettierPathOnlyRe = regexp.MustCompile(`^[^\s].*\.(js|jsx|ts|tsx|mjs|cjs|json|md|css|scss|yaml|yml|html|graphql)$`)
var prettierPathWithTimingRe = regexp.MustCompile(`^([^\s].*\.(js|jsx|ts|tsx|mjs|cjs|json|md|css|scss|yaml|yml|html|graphql))\s+\d+(\.\d+)?ms$`)

func summarizePrettierOutput(raw string) (string, bool) {
	state := prettierSummaryState{
		needsFormatting: make([]string, 0),
		formatted:       make([]string, 0),
		other:           make([]string, 0),
	}
	for _, line := range strings.Split(raw, "\n") {
		consumePrettierSummaryLine(&state, line)
	}
	return renderPrettierSummary(state)
}

func consumePrettierSummaryLine(state *prettierSummaryState, line string) {
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
		consumePrettierWarnLine(state, trimmed)
	case strings.HasPrefix(lower, "[error] "):
		state.other = append(state.other, trimmed)
	case prettierPathOnlyRe.MatchString(trimmed):
		state.needsFormatting = appendUnique(state.needsFormatting, trimmed)
	default:
		consumePrettierOtherLine(state, trimmed)
	}
}

func consumePrettierWarnLine(state *prettierSummaryState, trimmed string) {
	path := strings.TrimSpace(trimmed[len("[warn] "):])
	if prettierPathOnlyRe.MatchString(path) {
		state.needsFormatting = appendUnique(state.needsFormatting, path)
		return
	}
	state.other = append(state.other, trimmed)
}

func consumePrettierOtherLine(state *prettierSummaryState, trimmed string) {
	if m := prettierPathWithTimingRe.FindStringSubmatch(trimmed); len(m) > 1 {
		state.formatted = appendUnique(state.formatted, m[1])
		return
	}
	state.other = append(state.other, trimmed)
}

func renderPrettierSummary(state prettierSummaryState) (string, bool) {
	if len(state.needsFormatting) > 0 && state.hasCheckFailure {
		return renderPrettierCheckFailureSummary(state.needsFormatting), true
	}
	if state.hasCheckStart && state.hasCheckSuccess && len(state.needsFormatting) == 0 && len(state.other) == 0 {
		return "prettier check: ok\n", true
	}
	if len(state.formatted) > 1 && len(state.other) == 0 {
		return renderPrettierWriteSummary(state.formatted), true
	}
	return "", false
}

func renderPrettierCheckFailureSummary(paths []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("prettier check: %d files need formatting\n", len(paths)))
	for _, path := range paths {
		b.WriteString("- ")
		b.WriteString(path)
		b.WriteString("\n")
	}
	return b.String()
}

func renderPrettierWriteSummary(paths []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("prettier write: formatted %d files\n", len(paths)))
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

type prettierSummaryState struct {
	needsFormatting []string
	formatted       []string
	other           []string
	hasCheckStart   bool
	hasCheckSuccess bool
	hasCheckFailure bool
}
