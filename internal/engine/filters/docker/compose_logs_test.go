package dockerfilters

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const dockerComposeLogsTool = "docker compose logs"

func TestComposeLogsContextIsolationAndStderrImmediate(t *testing.T) {
	logs := NewComposeLogsFilter()
	k1 := logs.ContextKey(engine.Event{CommandID: "1", Tool: dockerComposeLogsTool, Dispatch: "docker compose logs|scope=api", Stream: engine.StdoutStream})
	k2 := logs.ContextKey(engine.Event{CommandID: "1", Tool: dockerComposeLogsTool, Dispatch: "docker compose logs|scope=web", Stream: engine.StdoutStream})
	if k1 == k2 {
		t.Fatalf("expected isolated context keys, got %q", k1)
	}
	d := logs.Process(engine.Event{Type: engine.EventLine, Tool: dockerComposeLogsTool, Dispatch: "docker compose logs|scope=api", Stream: engine.StderrStream, Line: "compose warning\n"}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionImmediate || d.Output != "compose warning\n" {
		t.Fatalf("expected immediate stderr visibility, got %#v", d)
	}
}

func TestComposeLogsProcessStdoutCollectionAndFlush(t *testing.T) {
	logs := NewComposeLogsFilter()
	mem := engine.NewOrderedSetBuffer()
	if got := logs.Process(engine.Event{Type: engine.EventLine, Tool: dockerComposeLogsTool, Dispatch: "docker compose logs|scope=api", Stream: engine.StdoutStream, Line: "api | line-a\n"}, mem); got.Action != engine.ActionCollect {
		t.Fatalf("expected stdout line collect, got %#v", got)
	}
	if got := logs.Process(engine.Event{Type: engine.EventTick, Tool: dockerComposeLogsTool, Dispatch: "docker compose logs|scope=api", Stream: engine.StdoutStream}, mem); got.Action != engine.ActionCollect {
		t.Fatalf("expected stdout tick collect, got %#v", got)
	}
	_ = mem.Add("api | line-a\n", "line-a\n", 1)
	_ = mem.Add("web | line-b\n", "line-b\n", 2)
	for _, eventType := range []engine.EventType{engine.EventEOF, engine.EventExit} {
		if got := logs.Process(engine.Event{Type: eventType, Tool: dockerComposeLogsTool, Dispatch: "docker compose logs|scope=all", Stream: engine.StdoutStream}, mem); got.Action != engine.ActionFlush || got.Output != "api | line-a\nweb | line-b\n" {
			t.Fatalf("expected %s raw flush, got %#v", eventType, got)
		}
	}
}

func TestComposeLogsProcessEmptyStdoutIgnored(t *testing.T) {
	logs := NewComposeLogsFilter()
	for _, eventType := range []engine.EventType{engine.EventEOF, engine.EventExit} {
		if got := logs.Process(engine.Event{Type: eventType, Tool: dockerComposeLogsTool, Dispatch: "docker compose logs|scope=all", Stream: engine.StdoutStream}, engine.NewOrderedSetBuffer()); got.Action != engine.ActionIgnore || got.Output != "" {
			t.Fatalf("expected empty %s ignore, got %#v", eventType, got)
		}
	}
}
