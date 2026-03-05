package gofilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const goBuildDispatch = "go build"

func TestBuildPrepareStructuredOutputPassthroughCases(t *testing.T) {
	f := NewBuildFilter()
	for _, args := range [][]string{{"-json"}, {"--json=stream"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			prep := f.Prepare(args)
			if !prep.ForcePassthrough || !prep.Ambiguous {
				t.Fatalf("expected structured passthrough for %v, got %#v", args, prep)
			}
			if prep.Reason != "structured output mode" {
				t.Fatalf("unexpected passthrough reason: %q", prep.Reason)
			}
		})
	}
}

func TestBuildCompaction(t *testing.T) {
	raw := strings.Join([]string{
		"go: downloading github.com/acme/foo v1.2.3",
		"pkg/main.go:10:2: undefined: missing",
	}, "\n") + "\n"
	out, ok := compactBuildVet(raw, goBuildDispatch)
	if !ok {
		t.Fatal("expected compact build output")
	}
	if !strings.Contains(out, "go build: 1 diagnostics") || !strings.Contains(out, "[info] downloading 1 dependencies...") {
		t.Fatalf("unexpected build output: %q", out)
	}
}

func TestBuildCompactionRecognizesTraceLines(t *testing.T) {
	raw := strings.Join([]string{
		"WORK=/tmp/go-build123",
		"cd /repo",
		"0.055s # cd /repo; git status --porcelain",
		"mkdir -p $WORK/b001/",
	}, "\n") + "\n"
	out, ok := compactBuildVet(raw, goBuildDispatch)
	if !ok {
		t.Fatal("expected compact build output for -x trace")
	}
	if !strings.Contains(out, "go build: ok") || !strings.Contains(out, "[info] go build trace lines: 4") {
		t.Fatalf("unexpected build trace output: %q", out)
	}
}

func TestBuildSubfilterStderrImmediate(t *testing.T) {
	f := NewBuildFilter()
	d := f.Process(engine.Event{Type: engine.EventLine, Tool: "go", Dispatch: goBuildDispatch, Stream: engine.StderrStream, Line: "build failed\n"}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionImmediate || d.Output != "build failed\n" {
		t.Fatalf("unexpected stderr decision: %#v", d)
	}
}

func TestBuildSubfilterStderrTraceCompaction(t *testing.T) {
	f := NewBuildFilter()
	mem := engine.NewOrderedSetBuffer()
	traceLine := "WORK=/tmp/go-build123\n"
	_ = mem.Add(traceLine, traceLine, 1)
	d := f.Process(engine.Event{
		Type:     engine.EventEOF,
		Tool:     "go",
		Dispatch: "go build|x=1",
		Stream:   engine.StderrStream,
	}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush for trace mode, got %#v", d)
	}
	if !strings.Contains(d.Output, "go build: ok") || !strings.Contains(d.Output, "[info] go build trace lines: 1") {
		t.Fatalf("unexpected trace compaction output: %q", d.Output)
	}
}

func TestBuildSubfilterStderrTraceEOFEmptyIgnores(t *testing.T) {
	f := NewBuildFilter()
	mem := engine.NewOrderedSetBuffer()
	d := f.Process(engine.Event{
		Type:     engine.EventEOF,
		Tool:     "go",
		Dispatch: "go build|x=1",
		Stream:   engine.StderrStream,
	}, mem)
	if d.Action != engine.ActionIgnore || d.Output != "" {
		t.Fatalf("expected ignore for empty trace buffer, got %#v", d)
	}
}

func TestBuildSubfilterExitHandling(t *testing.T) {
	f := NewBuildFilter()
	mem := engine.NewOrderedSetBuffer()

	eof := f.Process(engine.Event{Type: engine.EventEOF, Tool: "go", Dispatch: goBuildDispatch, Stream: engine.StdoutStream}, mem)
	if eof.Action != engine.ActionCollect {
		t.Fatalf("expected eof collect, got %#v", eof)
	}

	fail := f.Process(engine.Event{Type: engine.EventExit, Tool: "go", Dispatch: goBuildDispatch, Stream: engine.StdoutStream, ExitCode: 1}, mem)
	if fail.Action != engine.ActionIgnore {
		t.Fatalf("expected ignore for non-zero empty stdout, got %#v", fail)
	}

	ok := f.Process(engine.Event{Type: engine.EventExit, Tool: "go", Dispatch: goBuildDispatch, Stream: engine.StdoutStream, ExitCode: 0}, mem)
	if ok.Action != engine.ActionIgnore || ok.Output != "" {
		t.Fatalf("unexpected success decision: %#v", ok)
	}
}

func TestBuildSubfilterExitCompactionForBufferedStdout(t *testing.T) {
	f := NewBuildFilter()
	mem := engine.NewOrderedSetBuffer()
	line := "go: downloading github.com/acme/foo v1.2.3\n"
	_ = mem.Add(line, "k", 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Tool: "go", Dispatch: goBuildDispatch, Stream: engine.StdoutStream, ExitCode: 0}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush for buffered stdout at exit, got %#v", d)
	}
	if !strings.Contains(d.Output, "go build: ok") || !strings.Contains(d.Output, "[info] downloading 1 dependencies...") {
		t.Fatalf("unexpected compacted exit output: %q", d.Output)
	}
}

func TestBuildSubfilterFallbackToRawForBufferedOutput(t *testing.T) {
	f := NewBuildFilter()
	raw := "plain unrecognized line\n"
	cases := []struct {
		name  string
		event engine.Event
	}{
		{
			name: "stderr-trace-eof",
			event: engine.Event{
				Type:     engine.EventEOF,
				Tool:     "go",
				Dispatch: "go build|x=1",
				Stream:   engine.StderrStream,
			},
		},
		{
			name: "stdout-exit",
			event: engine.Event{
				Type:     engine.EventExit,
				Tool:     "go",
				Dispatch: goBuildDispatch,
				Stream:   engine.StdoutStream,
				ExitCode: 0,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			_ = mem.Add(raw, raw, 1)
			d := f.Process(tc.event, mem)
			if d.Action != engine.ActionFlush {
				t.Fatalf("expected flush fallback, got %#v", d)
			}
			if d.Output != raw {
				t.Fatalf("expected raw fallback output, got %q", d.Output)
			}
		})
	}
}
