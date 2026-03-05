package gitfilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const blameLinePorcelainRaw = "" +
	"60f15a314e2ccb08746086cb01886fb3b190d1e9 1 1 2\n" +
	"author bench\n" +
	"author-time 1704067200\n" +
	"author-tz +0000\n" +
	"committer bench\n" +
	"committer-time 1704067200\n" +
	"committer-tz +0000\n" +
	"filename tracked.txt\n" +
	"\tblame-line-1\n" +
	"60f15a314e2ccb08746086cb01886fb3b190d1e9 2 2\n" +
	"\tblame-line-2\n"

func TestGitBlameForcePassthrough(t *testing.T) {
	f := NewGitBlameFilter()
	got := f.Prepare([]string{"x"})
	if !got.ForcePassthrough {
		t.Fatalf("%T expected ForcePassthrough", f)
	}
}

func TestGitBlameLinePorcelainCompaction(t *testing.T) {
	f := NewGitBlameFilter()
	got := f.Prepare([]string{"--line-porcelain", "tracked.txt"})
	if got.ForcePassthrough {
		t.Fatalf("%T should compact --line-porcelain", f)
	}
	want := "" +
		"git blame: 2 lines\n" +
		"tracked.txt:1 author=bench @ 2024-01-01 00:00:00 +0000 committer=bench @ 2024-01-01 00:00:00 +0000 60f15a31 blame-line-1\n" +
		"tracked.txt:2 author=bench @ 2024-01-01 00:00:00 +0000 committer=bench @ 2024-01-01 00:00:00 +0000 60f15a31 blame-line-2\n"
	gotOut := compactBlameLinePorcelain(blameLinePorcelainRaw)
	if gotOut != want {
		t.Fatalf("output mismatch\nwant:\n%s\ngot:\n%s", want, gotOut)
	}
}

func TestGitBlameProcessSuccessCompactsOnExit(t *testing.T) {
	f := NewGitBlameFilter()
	mem := engine.NewOrderedSetBuffer()
	for i, line := range splitInputLines(blameLinePorcelainRaw) {
		mem.Add(line, line, uint64(i+1))
	}

	out := f.Process(engine.Event{Type: engine.EventExit, ExitCode: 0, Stream: engine.StdoutStream}, mem)
	if out.Action != engine.ActionFlush {
		t.Fatalf("expected success compaction flush, got %+v", out)
	}
	if !strings.Contains(out.Output, "git blame: 2 lines\n") {
		t.Fatalf("expected summary header, got:\n%s", out.Output)
	}
	if !strings.Contains(out.Output, "tracked.txt:1 author=bench") || !strings.Contains(out.Output, "committer=bench") {
		t.Fatalf("expected compact blame line fields retained, got:\n%s", out.Output)
	}
}

func TestGitBlameFailurePassthrough(t *testing.T) {
	f := NewGitBlameFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := "fatal: no such path 'missing.txt' in HEAD\n"
	for i, line := range splitInputLines(raw) {
		mem.Add(line, line, uint64(i+1))
	}
	out := f.Process(engine.Event{Type: engine.EventExit, ExitCode: 128, Stream: engine.StdoutStream}, mem)
	if out.Action != engine.ActionFlush || out.Output != raw {
		t.Fatalf("expected failure passthrough, got %+v", out)
	}
}

func splitInputLines(input string) []string {
	if input == "" {
		return nil
	}
	parts := strings.SplitAfter(input, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
