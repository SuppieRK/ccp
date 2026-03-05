package gitfilters

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestGitPushCases(t *testing.T) {
	f := NewGitPushFilter()
	cases := []struct {
		name       string
		raw        string
		exitCode   int
		wantAction engine.Action
		wantOutput string
	}{
		{name: "up-to-date", raw: "Everything up-to-date\n", exitCode: 0, wantAction: engine.ActionFlush, wantOutput: "Up-to-date\n"},
		{name: "ref-update-ok", raw: "To origin\n   abcdef0..1234567  main -> main\n", exitCode: 0, wantAction: engine.ActionFlush, wantOutput: "OK\n"},
		{name: "failure-raw-diagnostics", raw: "fatal: bad\n", exitCode: 1, wantAction: engine.ActionFlush, wantOutput: "fatal: bad\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			_ = mem.Add(tc.raw, tc.raw, 1)
			out := f.Process(engine.Event{Type: engine.EventExit, ExitCode: tc.exitCode}, mem)
			if out.Action != tc.wantAction {
				t.Fatalf("unexpected action: got %s want %s", out.Action, tc.wantAction)
			}
			if out.Output != tc.wantOutput {
				t.Fatalf("unexpected output: got %q want %q", out.Output, tc.wantOutput)
			}
		})
	}
}
