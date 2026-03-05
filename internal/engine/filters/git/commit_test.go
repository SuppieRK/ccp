package gitfilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestGitCommitFailurePreservesDiagnostics(t *testing.T) {
	f := NewGitCommitFilter()
	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add("fatal: bad\n", "k", 1)
	out := f.Process(engine.Event{Type: engine.EventExit, ExitCode: 1}, mem)
	if out.Action != engine.ActionFlush {
		t.Fatalf("expected flush on failure, got %s", out.Action)
	}
	if out.Output != "fatal: bad\n" {
		t.Fatalf("expected raw diagnostics, got %q", out.Output)
	}
}

func TestGitCommitSuccessRenderingCases(t *testing.T) {
	cases := []struct {
		name        string
		lines       []string
		wantExact   string
		wantContain string
	}{
		{
			name:        "compacts-hash",
			lines:       []string{"[main abcdef123] msg\n"},
			wantContain: "git commit: ok abcdef1",
		},
		{
			name: "includes-change-summary",
			lines: []string{
				"[main abcdef123] msg\n",
				" 1 file changed, 3 insertions(+)\n",
			},
			wantContain: "git commit: ok abcdef1 1 files +3 -0",
		},
		{
			name: "nothing-to-commit",
			lines: []string{
				"On branch main\n",
				"nothing to commit, working tree clean\n",
			},
			wantExact: "git commit: ok (nothing to commit)\n",
		},
		{
			name: "generic-success",
			lines: []string{
				"Enumerating objects: 3, done.\n",
				"Writing objects: 100% (3/3), 256 bytes | 256.00 KiB/s, done.\n",
			},
			wantExact: "git commit: ok\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := NewGitCommitFilter()
			mem := engine.NewOrderedSetBuffer()
			for i, line := range tc.lines {
				_ = mem.Add(line, line, uint64(i+1))
			}

			out := f.Process(engine.Event{Type: engine.EventExit, ExitCode: 0}, mem)
			if out.Action != engine.ActionFlush {
				t.Fatalf("expected flush, got %s", out.Action)
			}
			if tc.wantExact != "" && out.Output != tc.wantExact {
				t.Fatalf("expected exact output %q, got %q", tc.wantExact, out.Output)
			}
			if tc.wantContain != "" && !strings.Contains(out.Output, tc.wantContain) {
				t.Fatalf("expected output to contain %q, got %q", tc.wantContain, out.Output)
			}
		})
	}
}
