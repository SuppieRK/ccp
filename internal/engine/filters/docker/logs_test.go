package dockerfilters

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const dockerLogsTool = "docker logs"

func TestLogsContextIsolationAndStderrImmediate(t *testing.T) {
	logs := NewLogsFilter()
	k1 := logs.ContextKey(engine.Event{CommandID: "1", Tool: dockerLogsTool, Dispatch: "docker logs|container=api", Stream: engine.StdoutStream})
	k2 := logs.ContextKey(engine.Event{CommandID: "1", Tool: dockerLogsTool, Dispatch: "docker logs|container=web", Stream: engine.StdoutStream})
	if k1 == k2 {
		t.Fatalf("expected isolated context keys, got %q", k1)
	}
	d := logs.Process(engine.Event{Type: engine.EventLine, Tool: dockerLogsTool, Dispatch: "docker logs|container=api", Stream: engine.StderrStream, Line: "daemon warning\n"}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionImmediate || d.Output != "daemon warning\n" {
		t.Fatalf("expected immediate stderr visibility, got %#v", d)
	}
}

func TestLogsProcessStdoutCollectionAndFlush(t *testing.T) {
	logs := NewLogsFilter()
	mem := engine.NewOrderedSetBuffer()
	if got := logs.Process(engine.Event{Type: engine.EventLine, Tool: dockerLogsTool, Dispatch: "docker logs|container=api", Stream: engine.StdoutStream, Line: "line-a\n"}, mem); got.Action != engine.ActionCollect {
		t.Fatalf("expected stdout line collect, got %#v", got)
	}
	if got := logs.Process(engine.Event{Type: engine.EventTick, Tool: dockerLogsTool, Dispatch: "docker logs|container=api", Stream: engine.StdoutStream}, mem); got.Action != engine.ActionCollect {
		t.Fatalf("expected stdout tick collect, got %#v", got)
	}
	_ = mem.Add("line-a\n", "line-a\n", 1)
	_ = mem.Add("line-b\n", "line-b\n", 2)
	for _, eventType := range []engine.EventType{engine.EventEOF, engine.EventExit} {
		if got := logs.Process(engine.Event{Type: eventType, Tool: dockerLogsTool, Dispatch: "docker logs|container=api", Stream: engine.StdoutStream}, mem); got.Action != engine.ActionFlush || got.Output != "line-a\nline-b\n" {
			t.Fatalf("expected %s raw flush, got %#v", eventType, got)
		}
	}
}

func TestLogsProcessEmptyStdoutOnEOFAndExitIgnored(t *testing.T) {
	logs := NewLogsFilter()
	for _, eventType := range []engine.EventType{engine.EventEOF, engine.EventExit} {
		if got := logs.Process(engine.Event{Type: eventType, Tool: dockerLogsTool, Dispatch: "docker logs|container=api", Stream: engine.StdoutStream}, engine.NewOrderedSetBuffer()); got.Action != engine.ActionIgnore || got.Output != "" {
			t.Fatalf("expected empty %s ignore, got %#v", eventType, got)
		}
	}
}
