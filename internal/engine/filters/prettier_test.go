package filters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters/prettiercommon"
)

func TestPrettierToolMetadata(t *testing.T) {
	f := NewPrettierFilter()
	if f.Tool() != "prettier" {
		t.Fatalf("unexpected tool name: %q", f.Tool())
	}
	want := []string{"prettier.exe", "./prettier.exe", "prettier.cmd", "./prettier.cmd"}
	if !slices.Equal(f.Aliases(), want) {
		t.Fatalf("unexpected aliases: want=%#v got=%#v", want, f.Aliases())
	}
}

func TestPrettierPrepareSupportedModes(t *testing.T) {
	f := NewPrettierFilter()
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"--check", "src/good.js"}, want: "prettier|mode=check"},
		{args: []string{"--write", "src/write_bad.js"}, want: "prettier|mode=write"},
	}
	for _, tc := range tests {
		prep := f.Prepare(tc.args)
		if prep.ForcePassthrough {
			t.Fatalf("expected supported prepare for args=%#v", tc.args)
		}
		if prep.DispatchKey != tc.want {
			t.Fatalf("args=%#v expected dispatch=%q got=%q", tc.args, tc.want, prep.DispatchKey)
		}
		if !slices.Equal(prep.NormalizedArgs, tc.args) {
			t.Fatalf("expected args preserved, got %#v", prep.NormalizedArgs)
		}
	}
}

func TestPrettierPrepareUnsupportedShapesPassthrough(t *testing.T) {
	f := NewPrettierFilter()
	tests := [][]string{
		nil,
		{},
		{"src/good.js"},
		{"--check"},
		{"--write"},
		{"--check", "--write", "src/good.js"},
		{"--list-different", "src/good.js"},
		{"--check", "--ignore-unknown", "src/good.js"},
		{"--check", "--stdin-filepath", "src/good.js"},
	}
	for _, args := range tests {
		prep := f.Prepare(args)
		if !prep.ForcePassthrough {
			t.Fatalf("expected passthrough for args=%#v", args)
		}
	}
}

func TestPrettierSharedSummaries(t *testing.T) {
	checkFailure := strings.Join([]string{
		"Checking formatting...",
		"[warn] src/bad.js",
		"[warn] Code style issues found in the above file. Run Prettier with --write to fix.",
		"",
	}, "\n")
	out, ok := prettiercommon.SummarizeOutput(checkFailure)
	if !ok || !strings.Contains(out, "prettier check: 1 files need formatting") || !strings.Contains(out, "- src/bad.js") {
		t.Fatalf("unexpected check failure summary: ok=%v out=%q", ok, out)
	}

	checkSuccess := "Checking formatting...\nAll matched files use Prettier code style!\n"
	out, ok = prettiercommon.SummarizeOutput(checkSuccess)
	if !ok || out != "prettier check: ok\n" {
		t.Fatalf("unexpected check success summary: ok=%v out=%q", ok, out)
	}

	writeRaw := "src/bad.js 12ms\nsrc/other.ts 7ms\n"
	out, ok = prettiercommon.SummarizeOutput(writeRaw)
	if !ok || !strings.Contains(out, "prettier write: formatted 2 files") {
		t.Fatalf("unexpected write summary: ok=%v out=%q", ok, out)
	}
}

func TestPrettierFilterExitCases(t *testing.T) {
	f := NewPrettierFilter()
	cases := []struct {
		name       string
		raw        string
		wantAction engine.Action
		wantOutput string
	}{
		{
			name:       "check-success",
			raw:        "Checking formatting...\nAll matched files use Prettier code style!\n",
			wantAction: engine.ActionFlush,
			wantOutput: "prettier check: ok\n",
		},
		{
			name:       "fallback-on-unknown-output",
			raw:        "unexpected line one\nunexpected line two\n",
			wantAction: engine.ActionFlush,
			wantOutput: "unexpected line one\nunexpected line two\n",
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
