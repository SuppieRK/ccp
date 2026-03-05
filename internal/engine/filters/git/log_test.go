package gitfilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestGitLogPrepareCases(t *testing.T) {
	f := NewGitLogFilter()
	cases := []struct {
		name         string
		args         []string
		wantContains []string
		wantMissing  []string
		wantCount    map[string]int
	}{
		{
			name:         "default-format-limit-no-merges",
			args:         nil,
			wantContains: []string{"--pretty=format:%h %aI %an <%ae> | %s", "--no-merges"},
			wantCount:    map[string]int{"-10": 1},
		},
		{
			name:         "explicit-limit-format-and-merges-intent",
			args:         []string{"--oneline", "-25", "--merges"},
			wantContains: []string{"--oneline", "--merges"},
			wantMissing:  []string{"--pretty=format:%h %aI %an <%ae> | %s", "--no-merges"},
			wantCount:    map[string]int{"-25": 1},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			joined := strings.Join(prep.NormalizedArgs, " ")
			for _, want := range tc.wantContains {
				if !strings.Contains(joined, want) {
					t.Fatalf("expected %q in args, got %q", want, joined)
				}
			}
			for _, notWant := range tc.wantMissing {
				if strings.Contains(joined, notWant) {
					t.Fatalf("did not expect %q in args, got %q", notWant, joined)
				}
			}
			for token, want := range tc.wantCount {
				if got := strings.Count(joined, token); got != want {
					t.Fatalf("expected %q count %d, got %d (%q)", token, want, got, joined)
				}
			}
		})
	}
}

func TestGitLogStderrIsImmediate(t *testing.T) {
	f := NewGitLogFilter()
	out := f.Process(engine.Event{
		Type:   engine.EventLine,
		Tool:   "git log",
		Stream: engine.StderrStream,
		Line:   "fatal: boom\n",
	}, engine.NewOrderedSetBuffer())
	if out.Action != engine.ActionImmediate {
		t.Fatalf("expected immediate stderr passthrough, got %s", out.Action)
	}
	if out.Output != "fatal: boom\n" {
		t.Fatalf("unexpected stderr output: %q", out.Output)
	}
}

func TestGitLogEOFTruncatesLongLines(t *testing.T) {
	f := NewGitLogFilter()
	mem := engine.NewOrderedSetBuffer()
	long := strings.Repeat("x", 150) + "\n"
	short := "short-line\n"
	_ = mem.Add(long, "k1", 1)
	_ = mem.Add(short, "k2", 2)

	out := f.Process(engine.Event{
		Type:   engine.EventEOF,
		Tool:   "git log",
		Stream: engine.StdoutStream,
	}, mem)
	if out.Action != engine.ActionFlush {
		t.Fatalf("expected flush on eof, got %s", out.Action)
	}
	lines := strings.Split(strings.TrimSuffix(out.Output, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two output lines, got %d (%q)", len(lines), out.Output)
	}
	if len(lines[0]) != 120 {
		t.Fatalf("expected truncated line length 120, got %d", len(lines[0]))
	}
	if !strings.HasSuffix(lines[0], "...") {
		t.Fatalf("expected ellipsis on truncated line, got %q", lines[0])
	}
	if lines[1] != "short-line" {
		t.Fatalf("expected short line unchanged, got %q", lines[1])
	}
}
