package npxfilters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const (
	tscErrorLine  = "src/a.ts:1:1 error\n"
	tscFlagNoEmit = "--noEmit"
	tscFlagPretty = "--pretty"
)

func TestNpxTscFilterSuppressesWrapperNoise(t *testing.T) {
	f := NewNpxTscFilter()
	mem := engine.NewOrderedSetBuffer()
	for i, line := range []string{"Need to install the following packages:\n", "  tsc\n", "Ok to proceed? (y)\n", tscErrorLine} {
		_ = mem.Add(line, line, uint64(i+1))
	}
	d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream}, mem)
	if d.Action != engine.ActionFlush || strings.Contains(d.Output, "Need to install") || !strings.Contains(d.Output, "src/a.ts") {
		t.Fatalf("unexpected output: %#v", d)
	}
}

func TestNpxTscFilterStderrImmediate(t *testing.T) {
	f := NewNpxTscFilter()
	d := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "network error\n"}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionImmediate || d.Output != "network error\n" {
		t.Fatalf("expected immediate stderr passthrough, got %#v", d)
	}
}

func TestNpxTscFilterCollectsStdoutLineAndTick(t *testing.T) {
	f := NewNpxTscFilter()
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

func TestProcessRoutedIgnoresEOFToAvoidDoubleFlush(t *testing.T) {
	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add(tscErrorLine, tscErrorLine, 1)
	d := processRouted(engine.Event{Type: engine.EventEOF, Stream: engine.StdoutStream}, mem)
	if d.Action != engine.ActionIgnore {
		t.Fatalf("expected EOF ignore, got %#v", d)
	}
}

func TestNpxTscFilterExitIgnoresWrapperOnlyOrEmpty(t *testing.T) {
	f := NewNpxTscFilter()
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "wrapper-only",
			raw: strings.Join([]string{
				"Need to install the following packages:",
				"  typescript",
				"Ok to proceed? (y)",
				"npm WARN exec The following package was not found and will be installed",
			}, "\n") + "\n",
		},
		{
			name: "empty",
			raw:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			if tt.raw != "" {
				_ = mem.Add(tt.raw, tt.raw, 1)
			}
			d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream}, mem)
			if d.Action != engine.ActionIgnore || d.Output != "" {
				t.Fatalf("expected ignore for %s output, got %#v", tt.name, d)
			}
		})
	}
}

func TestSummarizeTSCOutputExpectedFormatContract(t *testing.T) {
	raw := strings.Join([]string{
		"src/fail.ts(1,7): error TS2322: Type 'string' is not assignable to type 'number'.",
		"src/fail.ts(4,7): error TS2322: Type 'string' is not assignable to type 'number'.",
		"src/fail2.ts(2,10): error TS2304: Cannot find name 'missingSymbol'.",
		"",
	}, "\n")
	got, ok := summarizeTSCOutput(raw)
	if !ok {
		t.Fatalf("expected summary")
	}
	want := strings.Join([]string{
		"src/fail.ts:",
		"- 1:7 error TS2322 Type 'string' is not assignable to type 'number'.",
		"- 4:7 error TS2322 Type 'string' is not assignable to type 'number'.",
		"src/fail2.ts:",
		"- 2:10 error TS2304 Cannot find name 'missingSymbol'.",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("unexpected formatted output\nwant:\n%s\n---\ngot:\n%s", want, got)
	}
	if strings.Contains(got, "tsc:") {
		t.Fatalf("unexpected global summary retained: %q", got)
	}
}

func TestNpxTscParseFallbackFlushesStrippedRaw(t *testing.T) {
	f := NewNpxTscFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := strings.Join([]string{
		"Need to install the following packages:",
		"  typescript",
		"Ok to proceed? (y)",
		"non-parseable payload line",
	}, "\n") + "\n"
	_ = mem.Add(raw, raw, 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush fallback for unparseable output, got %#v", d)
	}
	if d.Output != "non-parseable payload line\n" {
		t.Fatalf("expected stripped raw fallback output, got %q", d.Output)
	}
}

func TestNpxTscPrepareInjectsPrettyFalse(t *testing.T) {
	f := NewNpxTscFilter()
	prep := f.Prepare([]string{tscFlagNoEmit, "-p", "tsconfig.json"})
	got := prep.NormalizedArgs
	want := []string{tscFlagNoEmit, "-p", "tsconfig.json", tscFlagPretty, "false"}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected args: want=%v got=%v", want, got)
	}
}

func TestNpxTscPrepareKeepsExistingPrettyFlag(t *testing.T) {
	f := NewNpxTscFilter()
	prep := f.Prepare([]string{tscFlagNoEmit, tscFlagPretty, "true"})
	got := prep.NormalizedArgs
	want := []string{tscFlagNoEmit, tscFlagPretty, "true"}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected args: want=%v got=%v", want, got)
	}
}
