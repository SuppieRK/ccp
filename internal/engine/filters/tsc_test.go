package filters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestTscToolMetadata(t *testing.T) {
	f := NewTscFilter()
	if f.Tool() != "tsc" {
		t.Fatalf("unexpected tool name: %q", f.Tool())
	}
	want := []string{"tsc.exe", "./tsc.exe", "tsc.cmd", "./tsc.cmd"}
	if !slices.Equal(f.Aliases(), want) {
		t.Fatalf("unexpected aliases: want=%#v got=%#v", want, f.Aliases())
	}
}

func TestTscPrepareSupportedShapes(t *testing.T) {
	f := NewTscFilter()
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "inject pretty false",
			args: []string{"--noEmit", "-p", "tsconfig.json"},
			want: []string{"--noEmit", "-p", "tsconfig.json", "--pretty", "false"},
		},
		{
			name: "preserve explicit pretty false",
			args: []string{"--noEmit", "--pretty", "false"},
			want: []string{"--noEmit", "--pretty", "false"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if prep.ForcePassthrough {
				t.Fatalf("expected routed prepare for args=%#v", tc.args)
			}
			if prep.DispatchKey != "tsc" {
				t.Fatalf("unexpected dispatch key: %q", prep.DispatchKey)
			}
			if !slices.Equal(prep.NormalizedArgs, tc.want) {
				t.Fatalf("unexpected args: want=%#v got=%#v", tc.want, prep.NormalizedArgs)
			}
		})
	}
}

func TestTscPrepareUnsupportedShapesPassthrough(t *testing.T) {
	f := NewTscFilter()
	tests := [][]string{
		{"--pretty", "true", "--noEmit"},
		{"--watch"},
		{"-w"},
		{"--build"},
		{"--showConfig"},
		{"--diagnostics"},
	}
	for _, args := range tests {
		prep := f.Prepare(args)
		if !prep.ForcePassthrough {
			t.Fatalf("expected passthrough for args=%#v", args)
		}
	}
}

func TestTscFilterExitCases(t *testing.T) {
	f := NewTscFilter()
	cases := []struct {
		name       string
		raw        string
		wantAction engine.Action
		wantOutput string
	}{
		{
			name: "grouped diagnostics",
			raw: strings.Join([]string{
				"src/fail.ts(1,7): error TS2322: Type 'string' is not assignable to type 'number'.",
				"src/fail2.ts(2,10): error TS2304: Cannot find name 'missingSymbol'.",
				"",
			}, "\n"),
			wantAction: engine.ActionFlush,
			wantOutput: strings.Join([]string{
				"src/fail.ts:",
				"- 1:7 error TS2322 Type 'string' is not assignable to type 'number'.",
				"src/fail2.ts:",
				"- 2:10 error TS2304 Cannot find name 'missingSymbol'.",
				"",
			}, "\n"),
		},
		{
			name: "parseable but not shorter falls back raw",
			raw: strings.Join([]string{
				"src/fail.ts(1,7): error TS2322: Type 'string' is not assignable to type 'number'.",
				"",
			}, "\n"),
			wantAction: engine.ActionFlush,
			wantOutput: strings.Join([]string{
				"src/fail.ts(1,7): error TS2322: Type 'string' is not assignable to type 'number'.",
				"",
			}, "\n"),
		},
		{
			name:       "fallback raw output",
			raw:        "non-parseable payload line\n",
			wantAction: engine.ActionFlush,
			wantOutput: "non-parseable payload line\n",
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

func TestTscFilterStderrImmediate(t *testing.T) {
	f := NewTscFilter()
	d := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "tool missing\n"}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionImmediate || d.Output != "tool missing\n" {
		t.Fatalf("unexpected stderr decision: %#v", d)
	}
}
