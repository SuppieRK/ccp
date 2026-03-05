package cargofilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestCargoSubfilterMethodCoverage(t *testing.T) {
	t.Parallel()
	const expectedMaskingHorizon = 4096

	cases := []struct {
		name     string
		filter   engine.ToolFilter
		wantTool string
	}{
		{name: "build", filter: NewBuildFilter(), wantTool: "cargo build"},
		{name: "check", filter: NewCheckFilter(), wantTool: "cargo check"},
		{name: "clippy", filter: NewClippyFilter(), wantTool: "cargo clippy"},
		{name: "test", filter: NewTestFilter(), wantTool: "cargo test"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.filter.Tool(); got != tc.wantTool {
				t.Fatalf("Tool() = %q, want %q", got, tc.wantTool)
			}
			if got := tc.filter.Aliases(); got != nil {
				t.Fatalf("Aliases() = %v, want nil", got)
			}
			prep := tc.filter.Prepare([]string{"--release"})
			if len(prep.NormalizedArgs) != 1 || prep.NormalizedArgs[0] != "--release" {
				t.Fatalf("Prepare() normalized args mismatch: %#v", prep)
			}
			if tc.filter.ContextKey(engine.Event{CommandID: "c", Tool: tc.wantTool, Stream: engine.StdoutStream}) == "" {
				t.Fatal("ContextKey() returned empty key")
			}
			if got := tc.filter.MaskingHorizon(); got != expectedMaskingHorizon {
				t.Fatalf("MaskingHorizon() = %d, want %d", got, expectedMaskingHorizon)
			}
		})
	}
}

func TestCargoSubfilterProcessCoverage(t *testing.T) {
	t.Parallel()

	check := NewCheckFilter()
	clippy := NewClippyFilter()
	testf := NewTestFilter()

	mem := engine.NewOrderedSetBuffer()
	assertAction(t, "check line", check.Process(engine.Event{Type: engine.EventLine, Stream: engine.StdoutStream, Line: "line\n"}, mem), engine.ActionCollect)
	assertActionContains(t, "check eof", check.Process(engine.Event{Type: engine.EventEOF, Stream: engine.StderrStream}, mem), engine.ActionFlush, "cargo check: ok")

	mem = engine.NewOrderedSetBuffer()
	assertAction(t, "clippy line", clippy.Process(engine.Event{Type: engine.EventLine, Stream: engine.StdoutStream, Line: "line\n"}, mem), engine.ActionCollect)
	assertAction(t, "clippy tick", clippy.Process(engine.Event{Type: engine.EventTick, Stream: engine.StdoutStream}, mem), engine.ActionCollect)
	assertAction(t, "clippy eof", clippy.Process(engine.Event{Type: engine.EventEOF, Stream: engine.StderrStream}, mem), engine.ActionCollect)

	mem = engine.NewOrderedSetBuffer()
	assertAction(t, "test line", testf.Process(engine.Event{Type: engine.EventLine, Stream: engine.StdoutStream, Line: "line\n"}, mem), engine.ActionCollect)
	assertAction(t, "test tick", testf.Process(engine.Event{Type: engine.EventTick, Stream: engine.StdoutStream}, mem), engine.ActionCollect)
	assertAction(t, "test eof", testf.Process(engine.Event{Type: engine.EventEOF, Stream: engine.StdoutStream}, mem), engine.ActionIgnore)
	assertAction(t, "test exit", testf.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream, ExitCode: 1}, mem), engine.ActionIgnore)

	mem = engine.NewOrderedSetBuffer()
	_ = mem.Add("not-cargo-format\n", "not-cargo-format\n", 1)
	assertActionContains(t, "test fallback flush", testf.Process(engine.Event{Type: engine.EventEOF, Stream: engine.StdoutStream}, mem), engine.ActionFlush, "not-cargo-format")
	assertAction(t, "check exit", check.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream}, mem), engine.ActionIgnore)
	assertActionContains(t, "clippy exit", clippy.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream}, mem), engine.ActionFlush, "not-cargo-format")

	mem = engine.NewOrderedSetBuffer()
	d := clippy.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream}, mem)
	assertAction(t, "clippy empty-exit", d, engine.ActionIgnore)
	if d.Output != "" {
		t.Fatalf("clippy empty-exit expected empty output, got %q", d.Output)
	}
}

func TestCargoContextKeysIsolateCommandIDs(t *testing.T) {
	t.Parallel()

	build := NewBuildFilter()
	clippy := NewClippyFilter()

	buildA := build.ContextKey(engine.Event{CommandID: "cmd-a", Tool: "cargo build", Stream: engine.StdoutStream})
	buildB := build.ContextKey(engine.Event{CommandID: "cmd-b", Tool: "cargo build", Stream: engine.StdoutStream})
	assertDistinctNonEmptyKeys(t, "build context isolation", buildA, buildB)

	clippyA := clippy.ContextKey(engine.Event{CommandID: "cmd-a", Tool: "cargo clippy", Stream: engine.StdoutStream})
	clippyB := clippy.ContextKey(engine.Event{CommandID: "cmd-b", Tool: "cargo clippy", Stream: engine.StderrStream})
	assertDistinctNonEmptyKeys(t, "clippy context per command", clippyA, clippyB)

	clippySharedStdout := clippy.ContextKey(engine.Event{CommandID: "cmd-shared", Tool: "cargo clippy", Stream: engine.StdoutStream})
	clippySharedStderr := clippy.ContextKey(engine.Event{CommandID: "cmd-shared", Tool: "cargo clippy", Stream: engine.StderrStream})
	if clippySharedStdout == "" || clippySharedStderr == "" || clippySharedStdout != clippySharedStderr {
		t.Fatalf("clippy shared context across streams: expected equal non-empty keys, got %q vs %q", clippySharedStdout, clippySharedStderr)
	}
}

func assertAction(t *testing.T, label string, d engine.Decision, want engine.Action) {
	t.Helper()
	if d.Action != want {
		t.Fatalf("%s action = %q, want %q (%#v)", label, d.Action, want, d)
	}
}

func assertActionContains(t *testing.T, label string, d engine.Decision, want engine.Action, contains string) {
	t.Helper()
	assertAction(t, label, d, want)
	if !strings.Contains(d.Output, contains) {
		t.Fatalf("%s output missing %q: %q", label, contains, d.Output)
	}
}

func assertDistinctNonEmptyKeys(t *testing.T, label, a, b string) {
	t.Helper()
	if a == "" || b == "" || a == b {
		t.Fatalf("%s: expected distinct non-empty keys, got %q vs %q", label, a, b)
	}
}
