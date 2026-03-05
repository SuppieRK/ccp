package gitfilters

import (
	"slices"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestGitRebaseExitAware(t *testing.T) {
	f := NewGitRebaseFilter()
	cases := []struct {
		name       string
		exitCode   int
		lines      []string
		wantAction engine.Action
		wantOut    string
	}{
		{name: "success", exitCode: 0, wantAction: engine.ActionFlush, wantOut: "ok rebase\n"},
		{name: "failure-with-diagnostics", exitCode: 1, lines: []string{"conflict\n"}, wantAction: engine.ActionFlush, wantOut: "conflict\n"},
		{name: "failure-empty-buffer", exitCode: 1, wantAction: engine.ActionIgnore, wantOut: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			for i, line := range tc.lines {
				_ = mem.Add(line, line, uint64(i+1))
			}
			out := f.Process(engine.Event{Type: engine.EventExit, ExitCode: tc.exitCode}, mem)
			if out.Action != tc.wantAction {
				t.Fatalf("unexpected action for exit=%d: got %s want %s", tc.exitCode, out.Action, tc.wantAction)
			}
			if out.Output != tc.wantOut {
				t.Fatalf("unexpected output for exit=%d: got %q want %q", tc.exitCode, out.Output, tc.wantOut)
			}
		})
	}
}

func TestGitRebasePreparePassthrough(t *testing.T) {
	f := NewGitRebaseFilter()
	in := []string{"--continue"}
	prep := f.Prepare(in)
	if !slices.Equal(prep.NormalizedArgs, in) {
		t.Fatalf("Prepare() normalized args mismatch: got %#v want %#v", prep.NormalizedArgs, in)
	}
}
