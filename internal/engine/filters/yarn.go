package filters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
)

// NewYarnFilter returns the built-in Yarn passthrough filter.
func NewYarnFilter() engine.ToolFilter { return yarnFilter{} }

type yarnFilter struct{}

func (yarnFilter) Tool() string { return "yarn" }

func (yarnFilter) Aliases() []string {
	return []string{
		"yarnpkg",
		"yarn.cmd", "./yarn.cmd",
		"yarn.exe", "./yarn.exe",
		"yarnpkg.cmd", "./yarnpkg.cmd",
		"yarnpkg.exe", "./yarnpkg.exe",
	}
}

func (yarnFilter) Prepare(args []string) engine.PrepareResult {
	if len(args) == 0 {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}
	mode := "passthrough"
	if strings.EqualFold(strings.TrimSpace(args[0]), "run") {
		mode = "run"
	}
	return engine.PrepareResult{
		NormalizedArgs: append([]string{}, args...),
		DispatchKey:    "yarn|mode=" + mode,
	}
}

func (yarnFilter) ContextKey(ev engine.Event) string {
	// Yarn failures can split across streams; keep shared semantic context.
	return engine.SharedContextKey(ev)
}

func (yarnFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if d, ok := collectOnLineTickEOF(ev); ok {
		return d
	}
	switch ev.Type {
	case engine.EventExit:
		noOutputDecision := func() engine.Decision {
			if ev.ExitCode == 0 {
				return engine.Decision{Action: engine.ActionFlush, Output: "ok\n"}
			}
			return engine.Decision{Action: engine.ActionIgnore}
		}
		raw := mem.Joined()
		if strings.TrimSpace(raw) == "" {
			return noOutputDecision()
		}
		out, ok := compactNPMOutput(raw, parseNPMDispatch(ev.Dispatch), ev.ExitCode)
		if !ok {
			return engine.Decision{Action: engine.ActionFlush, Output: raw}
		}
		if strings.TrimSpace(out) == "" {
			if ev.ExitCode == 0 {
				return noOutputDecision()
			}
			return engine.Decision{Action: engine.ActionFlush, Output: raw}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: out}
	default:
		return engine.Decision{Action: engine.ActionIgnore}
	}
}

func (yarnFilter) MaskingHorizon() int { return 0 }
