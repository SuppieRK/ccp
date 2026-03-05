package kubectlfilters

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestLogsPrepareCases(t *testing.T) {
	f := NewKubectlLogsFilter()
	cases := []struct {
		name            string
		args            []string
		wantPassthrough bool
	}{
		{name: "follow-forces-passthrough", args: []string{"pod-1", "-f"}, wantPassthrough: true},
		{name: "non-follow-compacts", args: []string{"pod-1", "--tail", "100"}, wantPassthrough: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if prep.ForcePassthrough != tc.wantPassthrough {
				t.Fatalf("unexpected passthrough for %v: got %v want %v", tc.args, prep.ForcePassthrough, tc.wantPassthrough)
			}
		})
	}
}

func TestLogsStderrIsImmediate(t *testing.T) {
	f := NewKubectlLogsFilter()
	d := f.Process(engine.Event{Type: engine.EventLine, Tool: "kubectl", Dispatch: "kubectl logs", Stream: engine.StderrStream, Line: "warning\n"}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionImmediate {
		t.Fatalf("expected immediate action, got %q", d.Action)
	}
	if d.Output != "warning\n" {
		t.Fatalf("unexpected output: %q", d.Output)
	}
}

func TestLogsStdoutEOFCases(t *testing.T) {
	f := NewKubectlLogsFilter()
	cases := []struct {
		name       string
		lines      []string
		wantAction engine.Action
		wantOutput string
	}{
		{
			name:       "buffered-flushes",
			lines:      []string{"line one\n", "line two\n"},
			wantAction: engine.ActionFlush,
			wantOutput: "line one\nline two\n",
		},
		{
			name:       "empty-ignores",
			lines:      nil,
			wantAction: engine.ActionIgnore,
			wantOutput: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			for i, line := range tc.lines {
				_ = mem.Add(line, line, uint64(i+1))
			}
			d := f.Process(engine.Event{Type: engine.EventEOF, Tool: "kubectl", Dispatch: "kubectl logs", Stream: engine.StdoutStream}, mem)
			if d.Action != tc.wantAction {
				t.Fatalf("unexpected action: got %q want %q", d.Action, tc.wantAction)
			}
			if d.Output != tc.wantOutput {
				t.Fatalf("unexpected output: got %q want %q", d.Output, tc.wantOutput)
			}
		})
	}
}
