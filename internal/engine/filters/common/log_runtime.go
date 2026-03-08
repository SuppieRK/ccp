package common

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
)

type RawLogRuntimeConfig struct {
	FlushOnEOF  bool
	FlushOnExit bool
}

// ProcessRawLogs preserves stderr immediacy and flushes buffered stdout unchanged
// according to the configured EOF/Exit boundaries.
func ProcessRawLogs(ev engine.Event, mem *engine.OrderedSetBuffer, cfg RawLogRuntimeConfig) engine.Decision {
	if ev.Stream == engine.StderrStream {
		if ev.Type == engine.EventLine {
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		}
		return engine.Decision{Action: engine.ActionIgnore}
	}

	switch ev.Type {
	case engine.EventLine, engine.EventTick:
		return engine.Decision{Action: engine.ActionCollect}
	case engine.EventEOF:
		if !cfg.FlushOnEOF {
			return engine.Decision{Action: engine.ActionCollect}
		}
	case engine.EventExit:
		if !cfg.FlushOnExit {
			return engine.Decision{Action: engine.ActionCollect}
		}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}

	raw := mem.Joined()
	if strings.TrimSpace(raw) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: raw}
}
