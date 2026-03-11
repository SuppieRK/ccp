package filters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

func NewTscFilter() engine.ToolFilter { return tscFilter{} }

type tscFilter struct{}

func (tscFilter) Tool() string { return "tsc" }

func (tscFilter) Aliases() []string {
	return []string{"tsc.exe", "./tsc.exe", "tsc.cmd", "./tsc.cmd"}
}

func (tscFilter) MaskingHorizon() int { return 0 }

func (tscFilter) Prepare(args []string) engine.PrepareResult {
	normalized := append([]string{}, args...)
	prep := engine.PrepareResult{
		NormalizedArgs:   normalized,
		ForcePassthrough: true,
	}
	if !supportsDirectTSCShape(normalized) {
		return prep
	}
	if _, specified := filtercommon.TSCPrettyMode(normalized); !specified {
		normalized = append(normalized, "--pretty", "false")
	}
	prep.NormalizedArgs = normalized
	prep.ForcePassthrough = false
	prep.DispatchKey = "tsc"
	return prep
}

func (tscFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}

func (tscFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream == engine.StderrStream {
		switch ev.Type {
		case engine.EventLine:
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		default:
			return engine.Decision{Action: engine.ActionIgnore}
		}
	}
	switch ev.Type {
	case engine.EventTick, engine.EventLine:
		return engine.Decision{Action: engine.ActionCollect}
	case engine.EventEOF:
		return engine.Decision{Action: engine.ActionIgnore}
	case engine.EventExit:
		raw := mem.Joined()
		if strings.TrimSpace(raw) == "" {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		if summary, ok := filtercommon.SummarizeTSCOutput(raw); ok {
			if filtercommon.CountTSCDiagnosticFiles(raw) <= 1 && len(summary) >= len(raw) {
				return engine.Decision{Action: engine.ActionFlush, Output: raw}
			}
			return engine.Decision{Action: engine.ActionFlush, Output: summary}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

func supportsDirectTSCShape(args []string) bool {
	prettyEnabled, _ := filtercommon.TSCPrettyMode(args)
	if prettyEnabled {
		return false
	}
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		lower := strings.ToLower(trimmed)
		switch {
		case lower == "-w",
			lower == "--watch",
			lower == "--preservewatchoutput",
			lower == "-b",
			lower == "--build",
			lower == "-v",
			lower == "--version",
			lower == "-h",
			lower == "--help",
			lower == "--init",
			lower == "--showconfig",
			lower == "--listfiles",
			lower == "--listemittedfiles",
			lower == "--explainfiles",
			lower == "--diagnostics",
			lower == "--extendeddiagnostics",
			lower == "--traceresolution",
			strings.HasPrefix(lower, "--generatecpuprofile"),
			strings.HasPrefix(lower, "--generatetrace"):
			return false
		}
	}
	return true
}
