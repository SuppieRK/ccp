package kubectlfilters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
)

// NewKubectlLogsFilter compacts non-follow kubectl logs output.
func NewKubectlLogsFilter() engine.ToolFilter { return kubectlLogsFilter{} }

type kubectlLogsFilter struct{}

func (kubectlLogsFilter) Tool() string { return "kubectl logs" }

func (kubectlLogsFilter) Aliases() []string { return nil }

func (kubectlLogsFilter) Prepare(args []string) engine.PrepareResult {
	for _, arg := range args {
		if arg == "-f" || arg == "--follow" {
			return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
		}
	}
	return engine.PrepareResult{NormalizedArgs: args}
}

func (kubectlLogsFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}

func (kubectlLogsFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream == engine.StderrStream {
		if ev.Type == engine.EventLine {
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		}
		return engine.Decision{Action: engine.ActionIgnore}
	}
	switch ev.Type {
	case engine.EventEOF:
		raw := mem.Joined()
		if strings.TrimSpace(raw) == "" {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

func (kubectlLogsFilter) MaskingHorizon() int { return 4096 }
