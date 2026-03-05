package dockerfilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestImagesCompactionAndHeaderMismatch(t *testing.T) {
	cases := []struct {
		name         string
		raw          string
		wantContains string
	}{
		{
			name: "table",
			raw: strings.Join([]string{
				"REPOSITORY   TAG       IMAGE ID       CREATED       SIZE",
				"nginx        latest    abcdef         2 days ago    133MB",
				"redis        7         defabc         1 day ago     117MB",
			}, "\n") + "\n",
			wantContains: "nginx:latest [133MB]",
		},
		{
			name: "structured",
			raw: strings.Join([]string{
				"nginx:latest\t133MB",
				"redis:7\t117MB",
			}, "\n") + "\n",
			wantContains: "redis:7 [117MB]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, ok := compactImages(tc.raw, 15)
			if !ok {
				t.Fatalf("expected compact docker images output for %s", tc.name)
			}
			if !strings.Contains(out, "docker images: 2 images (250MB)") || !strings.Contains(out, tc.wantContains) {
				t.Fatalf("unexpected %s output: %q", tc.name, out)
			}
		})
	}

	if _, ok := compactImages("REPO TAG\nx y\n", 15); ok {
		t.Fatalf("expected header mismatch fallback")
	}
}

func TestImagesCompactionBoundedRowRendering(t *testing.T) {
	rawStructured := strings.Join([]string{
		"nginx:latest\t133MB",
		"redis:7\t117MB",
		"busybox:1\t5MB",
		"alpine:3\t7MB",
	}, "\n") + "\n"

	out, ok := compactImages(rawStructured, 2)
	if !ok {
		t.Fatal("expected compact docker images structured output")
	}
	if !strings.Contains(out, "docker images: 4 images (262MB)") {
		t.Fatalf("expected total summary, got %q", out)
	}
	if !strings.Contains(out, "nginx:latest [133MB]") || !strings.Contains(out, "redis:7 [117MB]") {
		t.Fatalf("expected first rows retained, got %q", out)
	}
	if !strings.Contains(out, "... +2 more") {
		t.Fatalf("expected bounded-row overflow marker, got %q", out)
	}
}

func TestImagesProcessRuntimeCollectionAndFlushParity(t *testing.T) {
	f := NewImagesFilter()
	mem := engine.NewOrderedSetBuffer()
	if got := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StdoutStream}, mem); got.Action != engine.ActionCollect {
		t.Fatalf("expected stdout line collect, got %#v", got)
	}
	if got := f.Process(engine.Event{Type: engine.EventTick, Stream: engine.StdoutStream}, mem); got.Action != engine.ActionCollect {
		t.Fatalf("expected stdout tick collect, got %#v", got)
	}
	if got := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "daemon warning\n"}, mem); got.Action != engine.ActionImmediate {
		t.Fatalf("expected stderr immediate, got %#v", got)
	}
	for _, eventType := range []engine.EventType{engine.EventEOF, engine.EventExit} {
		if got := f.Process(engine.Event{Type: eventType, Stream: engine.StdoutStream}, engine.NewOrderedSetBuffer()); got.Action != engine.ActionIgnore {
			t.Fatalf("expected empty %s ignore, got %#v", eventType, got)
		}
	}
}

func TestImagesProcessFlushesBufferedOutputOnEOFAndExit(t *testing.T) {
	for _, eventType := range []engine.EventType{engine.EventEOF, engine.EventExit} {
		t.Run(string(eventType), func(t *testing.T) {
			f := NewImagesFilter()
			mem := engine.NewOrderedSetBuffer()
			raw := "nginx:latest\t133MB\nredis:7\t117MB\n"
			_ = mem.Add(raw, raw, 1)
			d := f.Process(engine.Event{Type: eventType, Stream: engine.StdoutStream}, mem)
			if d.Action != engine.ActionFlush {
				t.Fatalf("expected %s flush, got %#v", eventType, d)
			}
			if !strings.Contains(d.Output, "docker images: 2 images (250MB)") {
				t.Fatalf("expected compacted %s output, got %q", eventType, d.Output)
			}
		})
	}
}
