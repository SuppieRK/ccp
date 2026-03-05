package gitfilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestGitDiffPrepareForcePassthroughCases(t *testing.T) {
	f := NewGitDiffFilter()
	cases := []struct {
		name               string
		args               []string
		mustStripNoCompact bool
	}{
		{name: "escape-flag", args: []string{"--no-compact", "--stat"}, mustStripNoCompact: true},
		{name: "numstat", args: []string{"--numstat"}},
		{name: "shortstat", args: []string{"--shortstat"}},
		{name: "numstat-and-shortstat", args: []string{"--numstat", "--shortstat"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := f.Prepare(tc.args)
			if !got.ForcePassthrough {
				t.Fatalf("expected ForcePassthrough for args=%#v", tc.args)
			}
			if tc.mustStripNoCompact {
				for _, arg := range got.NormalizedArgs {
					if arg == "--no-compact" {
						t.Fatal("expected --no-compact to be stripped from underlying git args")
					}
				}
			}
		})
	}
}

func TestGitDiffCompactsDiffOutput(t *testing.T) {
	f := NewGitDiffFilter()
	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add("diff --git a/a.go b/a.go\n", "a", 1)
	_ = mem.Add("@@ -1,2 +1,2 @@\n", "b", 2)
	_ = mem.Add("-old\n", "c", 3)
	_ = mem.Add("+new\n", "d", 4)

	out := f.Process(engine.Event{Type: engine.EventEOF, Tool: "git diff", Stream: engine.StdoutStream}, mem)
	if out.Action != engine.ActionFlush {
		t.Fatalf("expected flush, got %s", out.Action)
	}
	if !strings.Contains(out.Output, "a.go  +1 -1") {
		t.Fatalf("expected file-level summary, got %q", out.Output)
	}
	if !strings.Contains(out.Output, "summary: 1 files changed, +1 -1") {
		t.Fatalf("expected summary line, got %q", out.Output)
	}
}

func TestGitDiffStderrLineImmediate(t *testing.T) {
	f := NewGitDiffFilter()
	out := f.Process(engine.Event{
		Type:   engine.EventLine,
		Tool:   "git diff",
		Stream: engine.StderrStream,
		Line:   "fatal: bad revision\n",
	}, engine.NewOrderedSetBuffer())
	if out.Action != engine.ActionImmediate {
		t.Fatalf("expected immediate stderr action, got %s", out.Action)
	}
	if out.Output != "fatal: bad revision\n" {
		t.Fatalf("expected unchanged stderr output, got %q", out.Output)
	}
}
