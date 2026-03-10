package dockerfilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestPSCompactionPriorityAndPortsFolding(t *testing.T) {
	raw := strings.Join([]string{
		"CONTAINER ID   IMAGE     COMMAND   CREATED   STATUS                     PORTS                           NAMES",
		"a1            nginx     \"nginx\"  1m ago    Up 1 minute               0.0.0.0:80->80/tcp             web-1",
		"b2            nginx     \"nginx\"  1m ago    Up 1 minute               :::80->80/tcp                  web-2",
		"c3            api       \"run\"    1m ago    Exited (1) 10 seconds ago -                               api-1",
		"d4            api       \"run\"    1m ago    Up 1 minute               0.0.0.0:8080->80/tcp           api-2",
		"e5            api       \"run\"    1m ago    Up 1 minute               0.0.0.0:9090->80/tcp           api-3",
	}, "\n") + "\n"

	out, ok := compactPS(raw, 15)
	if !ok {
		t.Fatal("expected compact docker ps output")
	}
	if !strings.Contains(out, "[!] c3 api-1 api status=Exited (1) 10 seconds ago") {
		t.Fatalf("expected non-healthy row prioritized, got:\n%s", out)
	}
	if strings.Contains(out, "docker ps:") {
		t.Fatalf("expected no docker ps summary, got:\n%s", out)
	}
	if !strings.Contains(out, "[ok x2] nginx status=Up 1 minute") || !strings.Contains(out, "names=web-1,web-2") {
		t.Fatalf("expected healthy row grouping, got:\n%s", out)
	}
}

func TestPSHeaderMismatchFallsBack(t *testing.T) {
	raw := "SOMETHING ELSE\nfoo bar baz\n"
	if _, ok := compactPS(raw, 15); ok {
		t.Fatalf("expected header mismatch fallback")
	}
}

func TestPSCompactionBoundedRowRendering(t *testing.T) {
	raw := strings.Join([]string{
		"CONTAINER ID   IMAGE     COMMAND   CREATED   STATUS                     PORTS                 NAMES",
		"a1            nginx     \"nginx\"  1m ago    Up 1 minute               0.0.0.0:80->80/tcp   web-1",
		"b2            nginx     \"nginx\"  1m ago    Up 1 minute               0.0.0.0:80->80/tcp   web-2",
		"c3            nginx     \"nginx\"  1m ago    Exited (1) 10 sec ago     -                     api-1",
	}, "\n") + "\n"
	out, ok := compactPS(raw, 1)
	if !ok {
		t.Fatal("expected compact docker ps output")
	}
	if !strings.Contains(out, "... +2 more") {
		t.Fatalf("expected bounded overflow marker, got:\n%s", out)
	}
}

func TestPSProcessRuntimeCollectionAndFlushParity(t *testing.T) {
	f := NewPSFilter()
	mem := engine.NewOrderedSetBuffer()
	if got := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StdoutStream}, mem); got.Action != engine.ActionCollect {
		t.Fatalf("expected stdout line collect, got %#v", got)
	}
	if got := f.Process(engine.Event{Type: engine.EventTick, Stream: engine.StdoutStream}, mem); got.Action != engine.ActionCollect {
		t.Fatalf("expected stdout tick collect, got %#v", got)
	}
	if got := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "daemon error\n"}, mem); got.Action != engine.ActionImmediate {
		t.Fatalf("expected stderr immediate, got %#v", got)
	}
	for _, eventType := range []engine.EventType{engine.EventEOF, engine.EventExit} {
		if got := f.Process(engine.Event{Type: eventType, Stream: engine.StdoutStream}, engine.NewOrderedSetBuffer()); got.Action != engine.ActionIgnore {
			t.Fatalf("expected empty %s ignore, got %#v", eventType, got)
		}
	}
}

func TestPSProcessFlushesOnEOFAndExitWithBufferedOutput(t *testing.T) {
	for _, eventType := range []engine.EventType{engine.EventEOF, engine.EventExit} {
		t.Run(string(eventType), func(t *testing.T) {
			f := NewPSFilter()
			raw := strings.Join([]string{
				"CONTAINER ID   IMAGE     COMMAND   CREATED   STATUS                     PORTS                           NAMES",
				"a1            nginx     \"nginx\"  1m ago    Up 1 minute               0.0.0.0:80->80/tcp             web-1",
			}, "\n") + "\n"
			mem := engine.NewOrderedSetBuffer()
			_ = mem.Add(raw, raw, 1)
			if got := f.Process(engine.Event{Type: eventType, Stream: engine.StdoutStream}, mem); got.Action != engine.ActionFlush || !strings.Contains(got.Output, "[ok] nginx status=Up 1 minute") {
				t.Fatalf("expected %s flush with compacted output, got %#v", eventType, got)
			}
		})
	}
}

func TestPSCompactionOmitsCountMarkerForSingleHealthyRow(t *testing.T) {
	raw := strings.Join([]string{
		"CONTAINER ID   IMAGE     COMMAND   CREATED   STATUS       PORTS                 NAMES",
		"a1            nginx     \"nginx\"  1m ago    Up 1 minute  0.0.0.0:80->80/tcp   web-1",
	}, "\n") + "\n"
	out, ok := compactPS(raw, 15)
	if !ok {
		t.Fatal("expected compact docker ps output")
	}
	if strings.Contains(out, "[ok x1]") {
		t.Fatalf("expected x1 marker omitted, got:\n%s", out)
	}
	if !strings.Contains(out, "name=web-1") {
		t.Fatalf("expected singular name field, got:\n%s", out)
	}
}
