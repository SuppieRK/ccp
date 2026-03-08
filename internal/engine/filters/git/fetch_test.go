package gitfilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestGitFetchPrepareDetailPassthrough(t *testing.T) {
	f := NewGitFetchFilter()
	tests := []struct {
		name string
		args []string
	}{
		{name: "verbose", args: []string{"--verbose"}},
		{name: "dry-run", args: []string{"--dry-run", "origin"}},
		{name: "porcelain", args: []string{"--porcelain"}},
		{name: "escape-flag", args: []string{"--no-compact", "origin"}},
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

func TestGitFetchExitSummaryAndFallbackCases(t *testing.T) {
	f := NewGitFetchFilter()

	t.Run("summary", func(t *testing.T) {
		mem := engine.NewOrderedSetBuffer()
		raw := strings.Join([]string{
			"From /tmp/remote",
			" * [new branch]      feature-x  -> origin/feature-x",
			"   c4b832f..6fe960e  main       -> origin/main",
			" * [new tag]         v2         -> v2",
			"",
		}, "\n")
		_ = mem.Add(raw, "fetch", 1)
		out := f.Process(engine.Event{Type: engine.EventExit, Tool: "git fetch", ExitCode: 0}, mem)
		if out.Action != engine.ActionFlush {
			t.Fatalf("expected flush, got %s", out.Action)
		}
		for _, want := range []string{"git fetch: ok 3 ref updates", "1 new branch", "1 new tag"} {
			if !strings.Contains(out.Output, want) {
				t.Fatalf("expected %q in output, got %q", want, out.Output)
			}
		}
	})

	t.Run("empty-success", func(t *testing.T) {
		out := f.Process(engine.Event{Type: engine.EventExit, Tool: "git fetch", ExitCode: 0}, engine.NewOrderedSetBuffer())
		if out.Action != engine.ActionIgnore {
			t.Fatalf("expected ignore for empty success, got %#v", out)
		}
	})

	t.Run("low-confidence-fallback", func(t *testing.T) {
		mem := engine.NewOrderedSetBuffer()
		raw := "From /tmp/remote\nunexpected transport line\n"
		_ = mem.Add(raw, "fetch", 1)
		out := f.Process(engine.Event{Type: engine.EventExit, Tool: "git fetch", ExitCode: 0}, mem)
		if out.Action != engine.ActionFlush {
			t.Fatalf("expected flush, got %#v", out)
		}
		if out.Output != raw {
			t.Fatalf("expected raw fallback, got %q", out.Output)
		}
	})

	t.Run("failure", func(t *testing.T) {
		mem := engine.NewOrderedSetBuffer()
		raw := "fatal: not a git repository: '.git'\n"
		_ = mem.Add(raw, "fetch", 1)
		out := f.Process(engine.Event{Type: engine.EventExit, Tool: "git fetch", ExitCode: 128}, mem)
		if out.Action != engine.ActionFlush || out.Output != raw {
			t.Fatalf("expected raw failure passthrough, got %#v", out)
		}
	})
}
