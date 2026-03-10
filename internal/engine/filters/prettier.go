package filters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters/prettiercommon"
)

func NewPrettierFilter() engine.ToolFilter { return prettierFilter{} }

type prettierFilter struct{}

func (prettierFilter) Tool() string { return "prettier" }

func (prettierFilter) Aliases() []string {
	return []string{"prettier.exe", "./prettier.exe", "prettier.cmd", "./prettier.cmd"}
}

func (prettierFilter) MaskingHorizon() int { return 0 }

func (prettierFilter) Prepare(args []string) engine.PrepareResult {
	prep := engine.PrepareResult{
		NormalizedArgs:   append([]string{}, args...),
		ForcePassthrough: true,
	}
	mode, pathCount, ok := classifyPrettierArgs(args)
	if !ok {
		return prep
	}
	if mode == "" || pathCount == 0 {
		return prep
	}
	prep.ForcePassthrough = false
	prep.DispatchKey = prettierDispatchKey(mode)
	return prep
}

func classifyPrettierArgs(args []string) (string, int, bool) {
	mode := ""
	pathCount := 0
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "" {
			continue
		}
		nextMode, ok := updatePrettierMode(mode, trimmed)
		if !ok {
			return "", 0, false
		}
		if nextMode != mode {
			mode = nextMode
			continue
		}
		if isUnsupportedPrettierFlag(trimmed) {
			return "", 0, false
		}
		pathCount++
	}
	return mode, pathCount, true
}

func updatePrettierMode(mode, arg string) (string, bool) {
	if arg != "--check" && arg != "--write" {
		return mode, true
	}
	if mode != "" && mode != arg {
		return "", false
	}
	return arg, true
}

func isUnsupportedPrettierFlag(arg string) bool {
	return strings.HasPrefix(arg, "-")
}

func prettierDispatchKey(mode string) string {
	if mode == "--check" {
		return "prettier|mode=check"
	}
	return "prettier|mode=write"
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
		if summary, ok := prettiercommon.SummarizeOutput(raw); ok {
			return engine.Decision{Action: engine.ActionFlush, Output: summary}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}
