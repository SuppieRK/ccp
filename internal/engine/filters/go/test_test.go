package gofilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const goTestFailLine = "--- FAIL: TestX (0.00s)\n"

func TestPrepareStructuredOutputPassthrough(t *testing.T) {
	f := NewTestFilter()
	prep := f.Prepare([]string{"./...", "-json"})
	if !prep.ForcePassthrough || !prep.Ambiguous {
		t.Fatalf("expected structured passthrough, got %#v", prep)
	}
	if prep.Reason != "structured output mode" {
		t.Fatalf("unexpected passthrough reason: %q", prep.Reason)
	}
}

func TestCompactStatusFoldingAndPackageDedupe(t *testing.T) {
	raw := strings.Join([]string{
		"ok   github.com/acme/p1 0.101s",
		"ok   github.com/acme/p2 0.201s",
		"ok   github.com/acme/p1 0.099s",
		"?    github.com/acme/p3 [no test files]",
	}, "\n") + "\n"
	out, ok := compactTest(raw)
	if !ok {
		t.Fatal("expected compact output")
	}
	if !strings.Contains(out, "go test: 2 passed, 1 no-test-files") {
		t.Fatalf("unexpected summary: %q", out)
	}
}

func TestFailureCollectThenCompact(t *testing.T) {
	f := NewTestFilter()
	mem := engine.NewOrderedSetBuffer()
	d := f.Process(engine.Event{Type: engine.EventLine, Tool: "go", Dispatch: "go test", Stream: engine.StdoutStream, Line: goTestFailLine}, mem)
	if d.Action != engine.ActionCollect {
		t.Fatalf("expected collect on fail marker, got %q", d.Action)
	}
	_ = mem.Add(goTestFailLine, goTestFailLine, 1)
	_ = mem.Add("pkg/main_test.go:10: expected 1 got 2\n", "pkg/main_test.go:10: expected 1 got 2\n", 2)
	_ = mem.Add("FAIL\tgithub.com/acme/p1\t0.020s\n", "FAIL\tgithub.com/acme/p1\t0.020s\n", 3)
	d = f.Process(engine.Event{Type: engine.EventEOF, Tool: "go", Dispatch: "go test", Stream: engine.StdoutStream}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush at eof, got %q", d.Action)
	}
	if !strings.Contains(d.Output, "go test: 0 passed, 1 failed, 0 no-test-files") {
		t.Fatalf("expected compact failure summary, got %q", d.Output)
	}
	if strings.Count(d.Output, strings.TrimSuffix(goTestFailLine, "\n")) != 1 {
		t.Fatalf("expected no duplicated fail marker, got %q", d.Output)
	}
}

func TestLowConfidenceFallback(t *testing.T) {
	if _, ok := compactTest("abc\x00def\n"); ok {
		t.Fatal("expected low-confidence fallback")
	}
}

func TestGoTestProcessStderrLineImmediate(t *testing.T) {
	f := NewTestFilter()
	mem := engine.NewOrderedSetBuffer()
	ev := engine.Event{Type: engine.EventLine, Tool: "go", Dispatch: "go test", Stream: engine.StderrStream, Line: "warning: stderr\n"}
	d := f.Process(ev, mem)
	if d.Action != engine.ActionImmediate {
		t.Fatalf("expected immediate stderr action, got %q", d.Action)
	}
	if d.Output != ev.Line {
		t.Fatalf("expected unchanged stderr line, got %q", d.Output)
	}
}

func TestGoTestProcessEmptyStdoutBufferIgnoresEOFAndExit(t *testing.T) {
	f := NewTestFilter()
	for _, ev := range []engine.Event{
		{Type: engine.EventEOF, Tool: "go", Dispatch: "go test", Stream: engine.StdoutStream},
		{Type: engine.EventExit, Tool: "go", Dispatch: "go test", Stream: engine.StdoutStream},
	} {
		d := f.Process(ev, engine.NewOrderedSetBuffer())
		if d.Action != engine.ActionIgnore || d.Output != "" {
			t.Fatalf("expected ignore with empty output for event=%v, got %#v", ev.Type, d)
		}
	}
}

func TestGoTestProcessFallbackFlushesRaw(t *testing.T) {
	f := NewTestFilter()
	cases := []struct {
		name  string
		raw   string
		event engine.Event
	}{
		{
			name: "parse-fallback",
			raw:  "plain unrecognized line\n",
			event: engine.Event{
				Type:     engine.EventEOF,
				Tool:     "go",
				Dispatch: "go test",
				Stream:   engine.StdoutStream,
			},
		},
		{
			name: "low-confidence-fallback",
			raw:  "abc\x00def\n",
			event: engine.Event{
				Type:     engine.EventExit,
				Tool:     "go",
				Dispatch: "go test",
				Stream:   engine.StdoutStream,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			_ = mem.Add(tc.raw, tc.raw, 1)
			d := f.Process(tc.event, mem)
			if d.Action != engine.ActionFlush {
				t.Fatalf("expected flush fallback action, got %q", d.Action)
			}
			if d.Output != tc.raw {
				t.Fatalf("expected raw fallback output, got %q", d.Output)
			}
		})
	}
}
