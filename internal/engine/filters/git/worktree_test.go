package gitfilters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestGitWorktreePrepareShapes(t *testing.T) {
	f := NewGitWorktreeFilter()
	cases := []struct {
		name            string
		args            []string
		wantPassthrough bool
	}{
		{name: "list", args: []string{"list"}},
		{name: "list verbose", args: []string{"list", "--verbose"}, wantPassthrough: true},
		{name: "list porcelain", args: []string{"list", "--porcelain"}, wantPassthrough: true},
		{name: "add", args: []string{"add", "../wt"}, wantPassthrough: true},
		{name: "prune", args: []string{"prune"}, wantPassthrough: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if prep.ForcePassthrough != tc.wantPassthrough {
				t.Fatalf("ForcePassthrough = %v, want %v", prep.ForcePassthrough, tc.wantPassthrough)
			}
		})
	}
}

func TestGitWorktreeListCompaction(t *testing.T) {
	f := NewGitWorktreeFilter()
	mem := engine.NewOrderedSetBuffer()
	wd := t.TempDir()
	resolvedWD := resolvedWorktreePath(wd)
	sibling := filepath.Join(filepath.Dir(resolvedWD), "wt-feature")
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(resolvedWD); err != nil {
		t.Fatalf("chdir temp wd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})
	_ = mem.Add(resolvedWD+"  e77e6a2 [master]\n", "k1", 1)
	_ = mem.Add(sibling+"  e77e6a2 [feature]\n", "k2", 2)
	out := f.Process(engine.Event{Type: engine.EventEOF, Dispatch: "git worktree"}, mem)
	if out.Action != engine.ActionFlush {
		t.Fatalf("expected flush, got %s", out.Action)
	}
	for _, want := range []string{". e77e6a2 [master]", "../wt-feature e77e6a2 [feature]"} {
		if !strings.Contains(out.Output, want) {
			t.Fatalf("expected %q in %q", want, out.Output)
		}
	}
}

func TestGitWorktreeLowConfidenceFallback(t *testing.T) {
	f := NewGitWorktreeFilter()
	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add("/tmp/repo  e77e6a2 detached\n", "k1", 1)
	out := f.Process(engine.Event{Type: engine.EventEOF, Dispatch: "git worktree"}, mem)
	if out.Action != engine.ActionFlush {
		t.Fatalf("expected flush, got %s", out.Action)
	}
	if out.Output != "/tmp/repo  e77e6a2 detached\n" {
		t.Fatalf("expected raw fallback, got %q", out.Output)
	}
}
