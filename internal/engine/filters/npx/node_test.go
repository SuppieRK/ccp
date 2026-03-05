package npxfilters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestNpxNodeDelegatesCompaction(t *testing.T) {
	f := NewNpxNodeFilter()
	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add("(node:111) ExperimentalWarning: x\n", "w1", 1)
	_ = mem.Add("(node:222) ExperimentalWarning: x\n", "w2", 2)
	_ = mem.Add("payload\n", "payload", 3)
	d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream}, mem)
	if d.Action != engine.ActionFlush || strings.Count(d.Output, "ExperimentalWarning") != 1 || !strings.Contains(d.Output, "[+1 similar warnings]") {
		t.Fatalf("unexpected output: %#v", d)
	}
}

func TestNpxNodeMetadataAndPrepareSafety(t *testing.T) {
	f := NewNpxNodeFilter()
	if f.Tool() != "npx node" {
		t.Fatalf("unexpected tool: %q", f.Tool())
	}
	if f.Aliases() != nil {
		t.Fatalf("expected no aliases, got %#v", f.Aliases())
	}

	interactive := f.Prepare([]string{"-i"})
	if !interactive.ForcePassthrough {
		t.Fatalf("expected interactive passthrough, got %#v", interactive)
	}

	nonInteractive := f.Prepare([]string{"app.js"})
	if nonInteractive.ForcePassthrough {
		t.Fatalf("expected non-interactive to stay compactable, got %#v", nonInteractive)
	}
	if !slices.Equal(nonInteractive.NormalizedArgs, []string{"app.js"}) {
		t.Fatalf("unexpected normalized args: %#v", nonInteractive.NormalizedArgs)
	}
}

func TestNpxNodeContextKeySharedAcrossStreams(t *testing.T) {
	f := NewNpxNodeFilter()
	stdout := f.ContextKey(engine.Event{CommandID: "npx node app.js", Tool: "npx node", Stream: engine.StdoutStream})
	stderr := f.ContextKey(engine.Event{CommandID: "npx node app.js", Tool: "npx node", Stream: engine.StderrStream})
	if stdout != stderr {
		t.Fatalf("expected shared node context key, got %q != %q", stdout, stderr)
	}
}

func TestNpxNodeWrapperNoiseLineIgnored(t *testing.T) {
	f := NewNpxNodeFilter()
	for _, line := range []string{
		"Need to install the following packages:\n",
		"Ok to proceed? (y)\n",
		"npm WARN exec The following package was not found and will be installed\n",
	} {
		d := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StdoutStream, Line: line}, engine.NewOrderedSetBuffer())
		if d.Action != engine.ActionIgnore {
			t.Fatalf("expected wrapper-noise ignore for %q, got %#v", line, d)
		}
	}
}

func TestNpxNodeCollectsTickAndEOF(t *testing.T) {
	f := NewNpxNodeFilter()
	mem := engine.NewOrderedSetBuffer()
	for _, ev := range []engine.Event{
		{Type: engine.EventTick, Stream: engine.StdoutStream},
		{Type: engine.EventEOF, Stream: engine.StdoutStream},
	} {
		d := f.Process(ev, mem)
		if d.Action != engine.ActionCollect {
			t.Fatalf("expected collect for event=%v, got %#v", ev.Type, d)
		}
	}
}

func TestNpxNodeUnhandledFailureEarlyFlush(t *testing.T) {
	f := NewNpxNodeFilter()
	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add("(node:111) ExperimentalWarning: x\n", "w1", 1)
	_ = mem.Add("UnhandledPromiseRejectionWarning: boom\n", "u1", 2)
	d := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "UnhandledPromiseRejectionWarning: boom\n"}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected early flush on unhandled failure, got %#v", d)
	}
	if !strings.Contains(d.Output, "UnhandledPromiseRejectionWarning") {
		t.Fatalf("expected failure line in flushed output, got %q", d.Output)
	}
}

func TestNpxNodeExitCases(t *testing.T) {
	f := NewNpxNodeFilter()
	cases := []struct {
		name       string
		exitCode   int
		raw        string
		wantAction engine.Action
		wantOutput string
	}{
		{
			name:       "empty-buffer-ignores",
			exitCode:   0,
			raw:        "",
			wantAction: engine.ActionIgnore,
			wantOutput: "",
		},
		{
			name:       "low-confidence-fallback-raw",
			exitCode:   0,
			raw:        "abc\x00def\n",
			wantAction: engine.ActionFlush,
			wantOutput: "abc\x00def\n",
		},
		{
			name:       "nonzero-empty-compaction-fallback-raw",
			exitCode:   1,
			raw:        "\r⠋ loading...\n",
			wantAction: engine.ActionFlush,
			wantOutput: "\r⠋ loading...\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			if tc.raw != "" {
				_ = mem.Add(tc.raw, tc.raw, 1)
			}
			d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream, ExitCode: tc.exitCode}, mem)
			if d.Action != tc.wantAction || d.Output != tc.wantOutput {
				t.Fatalf("unexpected decision: got %#v", d)
			}
		})
	}
}
