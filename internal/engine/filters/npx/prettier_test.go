package npxfilters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters/prettiercommon"
)

func TestNpxPrettierToolName(t *testing.T) {
	if NewNpxPrettierFilter().Tool() != "npx prettier" {
		t.Fatal("unexpected tool name")
	}
}

func TestNpxPrettierPreparePreservesArgs(t *testing.T) {
	f := NewNpxPrettierFilter()
	in := []string{"--check", "src"}
	prep := f.Prepare(in)
	if !slices.Equal(prep.NormalizedArgs, in) {
		t.Fatalf("expected args preserved, got %#v", prep.NormalizedArgs)
	}
}

func TestNpxPrettierSummarizeCheckFailure(t *testing.T) {
	raw := strings.Join([]string{
		"Checking formatting...",
		"[warn] src/bad.js",
		"[warn] Code style issues found in the above file. Run Prettier with --write to fix.",
		"",
	}, "\n")
	out, ok := prettiercommon.SummarizeOutput(raw)
	if !ok {
		t.Fatalf("expected summarized output")
	}
	if !strings.Contains(out, "prettier check: 1 files need formatting") {
		t.Fatalf("unexpected summary: %q", out)
	}
	if !strings.Contains(out, "- src/bad.js") {
		t.Fatalf("missing file path in summary: %q", out)
	}
}

func TestNpxPrettierSummarizeCheckSuccess(t *testing.T) {
	raw := "Checking formatting...\nAll matched files use Prettier code style!\n"
	out, ok := prettiercommon.SummarizeOutput(raw)
	if !ok || out != "prettier check: ok\n" {
		t.Fatalf("unexpected success summary: ok=%v out=%q", ok, out)
	}
}

func TestNpxPrettierSummarizeWriteMode(t *testing.T) {
	raw := "src/bad.js 12ms\nsrc/other.ts 7ms\n"
	out, ok := prettiercommon.SummarizeOutput(raw)
	if !ok {
		t.Fatalf("expected summarized write output")
	}
	if !strings.Contains(out, "prettier write: formatted 2 files") {
		t.Fatalf("unexpected write summary: %q", out)
	}
}

func TestNpxPrettierFilterCollectsStdoutLineAndTick(t *testing.T) {
	f := NewNpxPrettierFilter()
	mem := engine.NewOrderedSetBuffer()
	for _, ev := range []engine.Event{
		{Type: engine.EventLine, Stream: engine.StdoutStream, Line: "payload\n"},
		{Type: engine.EventTick, Stream: engine.StdoutStream},
	} {
		d := f.Process(ev, mem)
		if d.Action != engine.ActionCollect {
			t.Fatalf("expected collect for event=%v, got %#v", ev.Type, d)
		}
	}
}

func TestNpxPrettierFilterStdoutEOFIgnores(t *testing.T) {
	f := NewNpxPrettierFilter()
	d := f.Process(engine.Event{Type: engine.EventEOF, Stream: engine.StdoutStream}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionIgnore || d.Output != "" {
		t.Fatalf("expected ignore/no output on stdout EOF, got %#v", d)
	}
}

func TestNpxPrettierFilterExitCases(t *testing.T) {
	f := NewNpxPrettierFilter()
	cases := []struct {
		name       string
		raw        string
		wantAction engine.Action
		wantOutput string
	}{
		{
			name:       "fallback-on-unknown-output",
			raw:        "unexpected line one\nunexpected line two\n",
			wantAction: engine.ActionFlush,
			wantOutput: "unexpected line one\nunexpected line two\n",
		},
		{
			name: "wrapper-only-ignores",
			raw: strings.Join([]string{
				"Need to install the following packages:",
				"  prettier@3.5.0",
				"Ok to proceed? (y)",
				"npm WARN exec The following package was not found and will be installed",
			}, "\n") + "\n",
			wantAction: engine.ActionIgnore,
			wantOutput: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			_ = mem.Add(tc.raw, tc.raw, 1)
			d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream}, mem)
			if d.Action != tc.wantAction || d.Output != tc.wantOutput {
				t.Fatalf("unexpected decision: got %#v", d)
			}
		})
	}
}
