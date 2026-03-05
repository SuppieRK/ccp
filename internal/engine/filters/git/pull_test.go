package gitfilters

import (
	"slices"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestGitPullSuccessCases(t *testing.T) {
	f := NewGitPullFilter()
	cases := []struct {
		name    string
		raw     string
		wantOut string
	}{
		{name: "up-to-date", raw: "Already up to date.\n", wantOut: "Up-to-date\n"},
		{name: "fast-forward", raw: "Updating 111..222\nFast-forward\n 2 files changed, 10 insertions(+), 1 deletion(-)\n", wantOut: "OK\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			_ = mem.Add(tc.raw, tc.raw, 1)
			out := f.Process(engine.Event{Type: engine.EventExit, ExitCode: 0}, mem)
			if out.Output != tc.wantOut {
				t.Fatalf("unexpected output: got %q want %q", out.Output, tc.wantOut)
			}
		})
	}
}

func TestGitPullPreparePassthrough(t *testing.T) {
	f := NewGitPullFilter()
	in := []string{"--ff-only", "origin", "main"}
	prep := f.Prepare(in)
	if !slices.Equal(prep.NormalizedArgs, in) {
		t.Fatalf("Prepare() normalized args mismatch: got %#v want %#v", prep.NormalizedArgs, in)
	}
}

func TestGitPullFailureCases(t *testing.T) {
	f := NewGitPullFilter()
	cases := []struct {
		name       string
		raw        string
		wantAction engine.Action
		wantOutput string
	}{
		{name: "with-diagnostics", raw: "fatal: bad\n", wantAction: engine.ActionFlush, wantOutput: "fatal: bad\n"},
		{name: "empty-buffer", raw: "", wantAction: engine.ActionIgnore, wantOutput: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			if tc.raw != "" {
				_ = mem.Add(tc.raw, tc.raw, 1)
			}
			out := f.Process(engine.Event{Type: engine.EventExit, ExitCode: 1}, mem)
			if out.Action != tc.wantAction {
				t.Fatalf("unexpected action: got %s want %s", out.Action, tc.wantAction)
			}
			if out.Output != tc.wantOutput {
				t.Fatalf("unexpected output: got %q want %q", out.Output, tc.wantOutput)
			}
		})
	}
}
