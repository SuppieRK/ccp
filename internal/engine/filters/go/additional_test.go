package gofilters

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestBuildPrepareHandlesLongJSONFlag(t *testing.T) {
	f := NewBuildFilter()
	cases := []struct {
		name            string
		args            []string
		wantPassthrough bool
		wantAmbiguous   bool
	}{
		{name: "json-flag", args: []string{"--json=stream"}, wantPassthrough: true, wantAmbiguous: true},
		{name: "plain-args", args: []string{"./..."}, wantPassthrough: false, wantAmbiguous: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if prep.ForcePassthrough != tc.wantPassthrough || prep.Ambiguous != tc.wantAmbiguous {
				t.Fatalf("unexpected prepare result for %v: %#v", tc.args, prep)
			}
		})
	}
}

func TestBuildProcessFlushesRawOnLowConfidenceFallback(t *testing.T) {
	f := NewBuildFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := "bad\x00payload\n"
	_ = mem.Add(raw, raw, 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream, Tool: "go build"}, mem)
	if d.Action != engine.ActionFlush || d.Output != raw {
		t.Fatalf("expected raw flush fallback, got %#v", d)
	}
}

func TestTestFilterProcessStderrNonLineIgnored(t *testing.T) {
	f := NewTestFilter()
	d := f.Process(engine.Event{Type: engine.EventTick, Stream: engine.StderrStream, Tool: "go test"}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionIgnore {
		t.Fatalf("expected stderr non-line ignore, got %#v", d)
	}
}
