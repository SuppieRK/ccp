package gitfilters

import (
	"slices"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const porcelainArg = "--porcelain"

func TestGitStatusPrepareCases(t *testing.T) {
	f := NewGitStatusFilter()
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{name: "defaults-to-porcelain", args: nil, want: []string{porcelainArg}},
		{name: "keeps-porcelain-arg", args: []string{porcelainArg, "-b"}, want: []string{porcelainArg, "-b"}},
		{name: "keeps-porcelain-equals-arg", args: []string{"--porcelain=v2", "-b"}, want: []string{"--porcelain=v2", "-b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := f.Prepare(tc.args)
			if !slices.Equal(got.NormalizedArgs, tc.want) {
				t.Fatalf("unexpected normalized args: got %#v want %#v", got.NormalizedArgs, tc.want)
			}
		})
	}
}

func TestGitStatusPassthroughOnEOF(t *testing.T) {
	f := NewGitStatusFilter()
	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add("## main...origin/main\n", "a", 1)
	_ = mem.Add("M  staged.go\n", "b", 2)
	_ = mem.Add(" M modified.go\n", "c", 3)
	_ = mem.Add("?? new.txt\n", "d", 4)

	out := f.Process(engine.Event{Type: engine.EventEOF, Tool: "git status", Stream: engine.StdoutStream}, mem)
	if out.Action != engine.ActionFlush {
		t.Fatalf("expected flush, got %s", out.Action)
	}
	if out.Output != "## main...origin/main\nM  staged.go\n M modified.go\n?? new.txt\n" {
		t.Fatalf("expected passthrough output, got %q", out.Output)
	}
}

func TestGitStatusStderrLineIsImmediate(t *testing.T) {
	f := NewGitStatusFilter()
	mem := engine.NewOrderedSetBuffer()
	ev := engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "fatal: bad config\n"}
	out := f.Process(ev, mem)
	if out.Action != engine.ActionImmediate {
		t.Fatalf("expected immediate, got %s", out.Action)
	}
	if out.Output != ev.Line {
		t.Fatalf("expected unchanged stderr line, got %q", out.Output)
	}
}

func TestGitStatusStdoutEOFEmptyBufferIgnores(t *testing.T) {
	f := NewGitStatusFilter()
	mem := engine.NewOrderedSetBuffer()
	out := f.Process(engine.Event{Type: engine.EventEOF, Stream: engine.StdoutStream}, mem)
	if out.Action != engine.ActionIgnore {
		t.Fatalf("expected ignore for empty stdout buffer at EOF, got %s", out.Action)
	}
	if out.Output != "" {
		t.Fatalf("expected empty output on ignore, got %q", out.Output)
	}
}
