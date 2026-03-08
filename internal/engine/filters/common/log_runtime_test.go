package common

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestProcessRawLogsStderrImmediate(t *testing.T) {
	t.Parallel()

	got := ProcessRawLogs(
		engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "warn\n"},
		engine.NewOrderedSetBuffer(),
		RawLogRuntimeConfig{FlushOnEOF: true, FlushOnExit: true},
	)
	if got.Action != engine.ActionImmediate || got.Output != "warn\n" {
		t.Fatalf("unexpected stderr decision: %#v", got)
	}
}

func TestProcessRawLogsStdoutFlushModes(t *testing.T) {
	t.Parallel()

	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add("line-a\n", "a", 1)
	_ = mem.Add("line-b\n", "b", 2)

	gotEOF := ProcessRawLogs(
		engine.Event{Type: engine.EventEOF, Stream: engine.StdoutStream},
		mem,
		RawLogRuntimeConfig{FlushOnEOF: true, FlushOnExit: false},
	)
	if gotEOF.Action != engine.ActionFlush || gotEOF.Output != "line-a\nline-b\n" {
		t.Fatalf("unexpected eof decision: %#v", gotEOF)
	}

	gotExit := ProcessRawLogs(
		engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream},
		mem,
		RawLogRuntimeConfig{FlushOnEOF: false, FlushOnExit: true},
	)
	if gotExit.Action != engine.ActionFlush || gotExit.Output != "line-a\nline-b\n" {
		t.Fatalf("unexpected exit decision: %#v", gotExit)
	}
}

func TestProcessRawLogsCollectsWhenFlushBoundaryDisabled(t *testing.T) {
	t.Parallel()

	mem := engine.NewOrderedSetBuffer()
	got := ProcessRawLogs(
		engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream},
		mem,
		RawLogRuntimeConfig{FlushOnEOF: true, FlushOnExit: false},
	)
	if got.Action != engine.ActionCollect {
		t.Fatalf("expected collect, got %#v", got)
	}
}
