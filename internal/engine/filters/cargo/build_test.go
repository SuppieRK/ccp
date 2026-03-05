package cargofilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const compilingLine = "Compiling app v0.1.0 (/repo)\n"

func TestCargoBuildSuppressionAndDiagnostics(t *testing.T) {
	raw := strings.Join([]string{
		"Fresh serde v1.0.214",
		"Compiling app v0.1.0 (/repo)",
		"error[E0425]: cannot find value `x` in this scope",
		"  --> src/main.rs:12:5",
	}, "\n") + "\n"
	out, ok := compactBuildCheck(raw, "cargo build")
	requireCompactBuildOutput(t, ok)
	if !strings.Contains(out, "cargo build: 2 diagnostics") {
		t.Fatalf("unexpected output: %q", out)
	}
	if strings.Contains(out, "[info] suppressed") {
		t.Fatalf("unexpected suppressed-progress line: %q", out)
	}
}

func TestCargoBuildNoDuplicateFlushOnExit(t *testing.T) {
	f := NewBuildFilter()
	mem := engine.NewOrderedSetBuffer()

	collect := f.Process(engine.Event{
		Type:   engine.EventLine,
		Tool:   "cargo",
		Stream: engine.StdoutStream,
		Line:   compilingLine,
	}, mem)
	assertDecisionCollectEmpty(t, collect)
	_ = mem.Add(compilingLine, compilingLine, 1)

	eof := f.Process(engine.Event{
		Type:   engine.EventEOF,
		Tool:   "cargo",
		Stream: engine.StdoutStream,
	}, mem)
	if eof.Action != engine.ActionFlush || !strings.Contains(eof.Output, "cargo build: ok") {
		t.Fatalf("unexpected eof decision: %#v", eof)
	}

	exit := f.Process(engine.Event{
		Type:   engine.EventExit,
		Tool:   "cargo",
		Stream: engine.StdoutStream,
	}, mem)
	assertDecisionIgnoreEmpty(t, exit)
}

func TestCargoBuildTickEventCollects(t *testing.T) {
	f := NewBuildFilter()
	mem := engine.NewOrderedSetBuffer()

	tick := f.Process(engine.Event{
		Type:   engine.EventTick,
		Tool:   "cargo",
		Stream: engine.StdoutStream,
	}, mem)
	assertDecisionCollectEmpty(t, tick)
}

func TestCargoBuildNoSyntheticSuccessOnExitWithEmptyStdout(t *testing.T) {
	f := NewBuildFilter()
	mem := engine.NewOrderedSetBuffer()
	d := f.Process(engine.Event{
		Type:     engine.EventExit,
		Tool:     "cargo",
		Stream:   engine.StdoutStream,
		ExitCode: 0,
	}, mem)
	assertDecisionIgnoreEmpty(t, d)
}

func TestCargoBuildEmptyEOFBufferIgnored(t *testing.T) {
	f := NewBuildFilter()
	mem := engine.NewOrderedSetBuffer()
	d := f.Process(engine.Event{
		Type:   engine.EventEOF,
		Tool:   "cargo",
		Stream: engine.StdoutStream,
	}, mem)
	assertDecisionIgnoreEmpty(t, d)
}

func TestCargoBuildDiagnosticsCapAndPriority(t *testing.T) {
	lines := []string{"Compiling app v0.1.0 (/repo)", "warning: warmup warning"}
	for i := 0; i < 21; i++ {
		lines = append(lines, "error[E0425]: cannot find value")
	}
	raw := strings.Join(lines, "\n") + "\n"
	out, ok := compactBuildCheck(raw, "cargo build")
	requireCompactBuildOutput(t, ok)
	if !strings.Contains(out, "cargo build: 22 diagnostics") || !strings.Contains(out, "... +2 more") {
		t.Fatalf("unexpected capped diagnostics output: %q", out)
	}
	parts := strings.Split(strings.TrimSpace(out), "\n")
	if len(parts) < 2 || !strings.Contains(parts[1], "error") {
		t.Fatalf("expected error-priority ordering, got %q", out)
	}
}

func TestCargoBuildLowConfidenceFallback(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "unrecognized output", raw: "hello from custom wrapper\n"},
		{name: "nul payload", raw: "error[E0425]: bad\x00line\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if out, ok := compactBuildCheck(tc.raw, "cargo build"); ok || out != "" {
				t.Fatalf("expected passthrough fallback, got out=%q ok=%v", out, ok)
			}
		})
	}
}

func assertDecisionIgnoreEmpty(t *testing.T, d engine.Decision) {
	t.Helper()
	if d.Action != engine.ActionIgnore || d.Output != "" {
		t.Fatalf("unexpected decision: %#v", d)
	}
}

func assertDecisionCollectEmpty(t *testing.T, d engine.Decision) {
	t.Helper()
	if d.Action != engine.ActionCollect || d.Output != "" {
		t.Fatalf("unexpected decision: %#v", d)
	}
}

func requireCompactBuildOutput(t *testing.T, ok bool) {
	t.Helper()
	if !ok {
		t.Fatal("expected compact output")
	}
}
