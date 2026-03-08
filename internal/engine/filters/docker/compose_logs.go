package dockerfilters

import (
	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

func NewComposeLogsFilter() engine.ToolFilter { return composeLogsFilter{} }

type composeLogsFilter struct{}

func (composeLogsFilter) Tool() string      { return "docker compose logs" }
func (composeLogsFilter) Aliases() []string { return nil }
func (composeLogsFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}
func (composeLogsFilter) ContextKey(ev engine.Event) string {
	scope := filtercommon.DispatchValue(ev.Dispatch, "scope")
	if scope == "" {
		scope = "all"
	}
	return ev.CommandID + ":" + ev.Tool + ":" + scope + ":" + string(ev.Stream)
}
func (composeLogsFilter) MaskingHorizon() int { return 4096 }

func (composeLogsFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	return filtercommon.ProcessRawLogs(ev, mem, filtercommon.RawLogRuntimeConfig{
		FlushOnEOF:  true,
		FlushOnExit: true,
	})
}
