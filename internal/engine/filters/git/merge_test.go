package gitfilters

import (
	"slices"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestGitMergeExitAware(t *testing.T) {
	f := NewGitMergeFilter()
	cases := []struct {
		name     string
		exitCode int
		lines    []string
		wantOut  string
	}{
		{name: "success", exitCode: 0, wantOut: "git merge: ok\n"},
		{name: "failure", exitCode: 1, lines: []string{"conflict\n"}, wantOut: "conflict\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			for i, line := range tc.lines {
				_ = mem.Add(line, line, uint64(i+1))
			}
			out := f.Process(engine.Event{Type: engine.EventExit, ExitCode: tc.exitCode}, mem)
			if out.Output != tc.wantOut {
				t.Fatalf("unexpected output for exit=%d: %q", tc.exitCode, out.Output)
			}
		})
	}
}

func TestGitMergePreparePassthrough(t *testing.T) {
	f := NewGitMergeFilter()
	in := []string{"--no-ff", "feature/x"}
	prep := f.Prepare(in)
	if !slices.Equal(prep.NormalizedArgs, in) {
		t.Fatalf("Prepare() normalized args mismatch: got %#v want %#v", prep.NormalizedArgs, in)
	}
}
