package filters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestPlaywrightPrepareCases(t *testing.T) {
	f := NewPlaywrightFilter()
	for _, tc := range []struct {
		name        string
		args        []string
		dispatch    string
		passthrough bool
	}{
		{name: "test dispatch", args: []string{"test"}, dispatch: playwrightDispatchKey},
		{name: "grep dispatch", args: []string{"test", "--grep", "auth"}, dispatch: playwrightDispatchKey},
		{name: "reporter passthrough", args: []string{"test", "--reporter=json"}, passthrough: true},
		{name: "show-report passthrough", args: []string{"show-report"}, passthrough: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if tc.passthrough != prep.ForcePassthrough {
				t.Fatalf("unexpected passthrough=%v for %#v", prep.ForcePassthrough, tc.args)
			}
			if prep.DispatchKey != tc.dispatch {
				t.Fatalf("unexpected dispatch=%q for %#v", prep.DispatchKey, tc.args)
			}
		})
	}
}

func TestCompactPlaywrightOutput(t *testing.T) {
	raw := strings.Join([]string{
		"Running 3 tests using 1 worker",
		"  ✓  1 tests/auth.spec.ts:3:1 › auth › logs in (1.1s)",
		"  ✘  2 tests/auth.spec.ts:8:1 › auth › rejects bad password (2.0s)",
		"    Error: expect(received).toBeTruthy()",
		"    screenshot: test-results/auth-failure.png",
		"  2 passed, 1 failed (3.1s)",
		"",
	}, "\n")
	out, ok := compactPlaywrightOutput(raw)
	if !ok {
		t.Fatal("expected compaction")
	}
	for _, want := range []string{"playwright: 2 passed, 1 failed (3.1s)", "failed tests:", "tests/auth.spec.ts:8:1", "Error: expect(received).toBeTruthy()"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestCompactPlaywrightFallback(t *testing.T) {
	if _, ok := compactPlaywrightOutput("random output\n"); ok {
		t.Fatal("expected fallback")
	}
}

func TestPlaywrightProcessStderrImmediate(t *testing.T) {
	f := NewPlaywrightFilter()
	d := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "fatal\n"}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionImmediate || d.Output != "fatal\n" {
		t.Fatalf("unexpected decision %#v", d)
	}
}
