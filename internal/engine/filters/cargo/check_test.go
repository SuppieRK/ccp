package cargofilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestCargoCheckSuccessBufferDrop(t *testing.T) {
	raw := strings.Join([]string{
		"Checking app v0.1.0 (/repo)",
		"Fresh serde v1.0.214",
		"Finished dev [unoptimized + debuginfo] target(s) in 0.12s",
	}, "\n") + "\n"
	out, ok := compactBuildCheck(raw, "cargo check")
	requireCompactCheckOutput(t, ok)
	if strings.TrimSpace(out) != "cargo check: ok" {
		t.Fatalf("expected check success summary, got %q", out)
	}
}

func TestCargoCheckDiagnosticsCap(t *testing.T) {
	lines := []string{"Checking app v0.1.0 (/repo)"}
	for i := 0; i < 21; i++ {
		lines = append(lines, "warning: unused import: `std::fmt`")
	}
	raw := strings.Join(lines, "\n") + "\n"
	out, ok := compactBuildCheck(raw, "cargo check")
	requireCompactCheckOutput(t, ok)
	if !strings.Contains(out, "cargo check: 21 diagnostics") || !strings.Contains(out, "... +1 more") {
		t.Fatalf("unexpected capped diagnostics output: %q", out)
	}
}

func TestCargoCheckDiagnosticsPriorityOrdering(t *testing.T) {
	raw := strings.Join([]string{
		"warning: this is a warning",
		"error[E0425]: cannot find value `x` in this scope",
	}, "\n") + "\n"
	out, ok := compactBuildCheck(raw, "cargo check")
	requireCompactCheckOutput(t, ok)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 || !strings.Contains(lines[1], "error") {
		t.Fatalf("expected error-priority ordering, got %q", out)
	}
}

func TestCargoCheckLowConfidenceFallback(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "unrecognized output", raw: "totally custom output\n"},
		{name: "nul payload", raw: "error[E0425]: bad\x00line\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if out, ok := compactBuildCheck(tc.raw, "cargo check"); ok || out != "" {
				t.Fatalf("expected passthrough fallback, got out=%q ok=%v", out, ok)
			}
		})
	}
}

func TestCargoCheckTickEventCollects(t *testing.T) {
	f := NewCheckFilter()
	mem := engine.NewOrderedSetBuffer()

	d := f.Process(engine.Event{Type: engine.EventTick, Tool: "cargo", Stream: engine.StdoutStream}, mem)
	assertDecisionCollectEmpty(t, d)
}

func TestCargoCheckEmptyStdoutEOFIgnored(t *testing.T) {
	f := NewCheckFilter()
	mem := engine.NewOrderedSetBuffer()

	d := f.Process(engine.Event{Type: engine.EventEOF, Tool: "cargo", Stream: engine.StdoutStream}, mem)
	assertDecisionIgnoreEmpty(t, d)
}

func TestCargoCheckEOFWithBufferedOutputFlushesSummary(t *testing.T) {
	f := NewCheckFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := "Checking app v0.1.0 (/repo)\n"
	_ = mem.Add(raw, raw, 1)

	d := f.Process(engine.Event{Type: engine.EventEOF, Tool: "cargo", Stream: engine.StdoutStream}, mem)
	if d.Action != engine.ActionFlush || !strings.Contains(d.Output, "cargo check: ok") {
		t.Fatalf("unexpected eof flush decision: %#v", d)
	}
}

func TestCargoCheckExitIgnored(t *testing.T) {
	f := NewCheckFilter()
	mem := engine.NewOrderedSetBuffer()

	d := f.Process(engine.Event{Type: engine.EventExit, Tool: "cargo", Stream: engine.StdoutStream, ExitCode: 3}, mem)
	assertDecisionIgnoreEmpty(t, d)
}

func requireCompactCheckOutput(t *testing.T, ok bool) {
	t.Helper()
	if !ok {
		t.Fatal("expected compact output")
	}
}
