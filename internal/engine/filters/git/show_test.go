package gitfilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestGitShowPreparePassthroughCases(t *testing.T) {
	f := NewGitShowFilter()
	tests := []struct {
		name string
		args []string
	}{
		{name: "format", args: []string{"--format=%H"}},
		{name: "pretty", args: []string{"--pretty=raw"}},
		{name: "stat", args: []string{"--stat"}},
		{name: "numstat", args: []string{"--numstat"}},
		{name: "blob", args: []string{"HEAD:README.md"}},
		{name: "escape-flag", args: []string{"--no-compact", "HEAD"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if !prep.ForcePassthrough {
				t.Fatalf("expected ForcePassthrough for %#v", tc.args)
			}
			for _, arg := range prep.NormalizedArgs {
				if arg == "--no-compact" {
					t.Fatalf("expected --no-compact to be stripped from %#v", prep.NormalizedArgs)
				}
			}
		})
	}
}

func TestGitShowCompactsDefaultOutput(t *testing.T) {
	f := NewGitShowFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := strings.Join([]string{
		"commit 05377dbf15f3ab2b35bef3df0d7d47c58da6d688",
		"Author: bench <bench@example.com>",
		"Date:   Sun Mar 8 08:17:08 2026 +0100",
		"",
		"    third commit",
		"",
		"diff --git a/tracked.txt b/tracked.txt",
		"index 8c1384d..29ef827 100644",
		"--- a/tracked.txt",
		"+++ b/tracked.txt",
		"@@ -1 +1 @@",
		"-v2",
		"+v3",
		"",
	}, "\n")
	_ = mem.Add(raw, "show", 1)

	out := f.Process(engine.Event{Type: engine.EventEOF, Tool: "git show", Stream: engine.StdoutStream}, mem)
	if out.Action != engine.ActionFlush {
		t.Fatalf("expected flush, got %s", out.Action)
	}
	for _, want := range []string{
		"commit 05377dbf15f3",
		"author: bench <bench@example.com>",
		"date: Sun Mar 8 08:17:08 2026 +0100",
		"subject: third commit",
		"tracked.txt  +1 -1",
		"summary: 1 files changed, +1 -1",
	} {
		if !strings.Contains(out.Output, want) {
			t.Fatalf("expected %q in compacted output, got %q", want, out.Output)
		}
	}
	if strings.Contains(out.Output, "diff --git") {
		t.Fatalf("expected diff header to be compacted away, got %q", out.Output)
	}
}

func TestGitShowLowConfidenceFallsBackToRaw(t *testing.T) {
	f := NewGitShowFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := strings.Join([]string{
		"commit 05377dbf15f3ab2b35bef3df0d7d47c58da6d688",
		"Author: bench <bench@example.com>",
		"Date:   Sun Mar 8 08:17:08 2026 +0100",
		"",
		"    merge commit",
		"",
		"diff --combined tracked.txt",
		"@@@ -1,1 -1,1 +1,1 @@@",
		"-v2",
		"+v3",
		"",
	}, "\n")
	_ = mem.Add(raw, "show", 1)

	out := f.Process(engine.Event{Type: engine.EventEOF, Tool: "git show", Stream: engine.StdoutStream}, mem)
	if out.Action != engine.ActionFlush {
		t.Fatalf("expected flush, got %s", out.Action)
	}
	if out.Output != raw {
		t.Fatalf("expected raw fallback, got %q", out.Output)
	}
}

func TestGitShowStderrLineImmediate(t *testing.T) {
	f := NewGitShowFilter()
	out := f.Process(engine.Event{
		Type:   engine.EventLine,
		Tool:   "git show",
		Stream: engine.StderrStream,
		Line:   "fatal: bad revision\n",
	}, engine.NewOrderedSetBuffer())
	if out.Action != engine.ActionImmediate {
		t.Fatalf("expected immediate stderr action, got %s", out.Action)
	}
	if out.Output != "fatal: bad revision\n" {
		t.Fatalf("unexpected stderr output: %q", out.Output)
	}
}
