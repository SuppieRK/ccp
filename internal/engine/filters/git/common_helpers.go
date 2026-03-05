package gitfilters

import (
	"strconv"
	"strings"

	"go-command-compression-proxy/internal/engine"
)

func sharedContextKey(ev engine.Event) string {
	return engine.SharedContextKey(ev)
}

func passthroughOnExit(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	return processExit(ev, mem, func(raw string) string { return raw })
}

func genericWriteProcess(ev engine.Event, mem *engine.OrderedSetBuffer, success string) engine.Decision {
	return processExit(ev, mem, func(_ string) string { return success + "\n" })
}

func processExit(ev engine.Event, mem *engine.OrderedSetBuffer, onSuccess func(raw string) string) engine.Decision {
	if ev.Type != engine.EventExit {
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := mem.Joined()
	if ev.ExitCode != 0 {
		return flushRawOrIgnore(raw)
	}
	out := onSuccess(raw)
	if strings.TrimSpace(out) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}

func flushRawOrIgnore(raw string) engine.Decision {
	if raw == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: raw}
}

func extractChangeSummary(raw string) (files int, adds int, dels int) {
	for _, line := range strings.Split(raw, "\n") {
		if !strings.Contains(line, "file") || !strings.Contains(line, "changed") {
			continue
		}
		for _, part := range strings.Split(line, ",") {
			part = strings.TrimSpace(part)
			switch {
			case strings.Contains(part, "file"):
				files = firstInt(part)
			case strings.Contains(part, "insertion"):
				adds = firstInt(part)
			case strings.Contains(part, "deletion"):
				dels = firstInt(part)
			}
		}
		if files > 0 {
			return files, adds, dels
		}
	}
	return 0, 0, 0
}

func firstInt(s string) int {
	for _, part := range strings.Fields(s) {
		if n, err := strconv.Atoi(part); err == nil {
			return n
		}
	}
	return 0
}
