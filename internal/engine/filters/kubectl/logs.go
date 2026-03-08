package kubectlfilters

import (
	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
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
	return filtercommon.ProcessRawLogs(ev, mem, filtercommon.RawLogRuntimeConfig{
		FlushOnEOF:  true,
		FlushOnExit: false,
	})
}

func (kubectlLogsFilter) MaskingHorizon() int { return 4096 }
