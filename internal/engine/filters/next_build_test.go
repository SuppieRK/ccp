package filters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestNextBuildPrepareCases(t *testing.T) {
	f := NewNextBuildFilter()
	for _, tc := range []struct {
		name        string
		args        []string
		dispatch    string
		passthrough bool
	}{
		{name: "build dispatch", args: []string{"build"}, dispatch: nextBuildDispatchKey},
		{name: "build path dispatch", args: []string{"build", "--no-lint"}, dispatch: nextBuildDispatchKey},
		{name: "debug passthrough", args: []string{"build", "--debug"}, passthrough: true},
		{name: "non-build passthrough", args: []string{"dev"}, passthrough: true},
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

func TestSummarizeNextBuild(t *testing.T) {
	raw := strings.Join([]string{
		"▲ Next.js 15.2.0",
		"Creating an optimized production build ...",
		"✓ Compiled successfully",
		"Route (app)                    Size     First Load JS",
		"├ ○ /                          1.2 kB        132 kB",
		"└ ● /dashboard                 2.5 kB        156 kB",
		"✓ Built in 34.2s",
		"",
	}, "\n")
	out, ok := summarizeNextBuild(raw)
	if !ok {
		t.Fatal("expected summary")
	}
	for _, want := range []string{"next build: success", "time: 34.2s", "routes: 2 total", "static / 132.0 kB", "dynamic /dashboard 156.0 kB"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestSummarizeNextBuildFallback(t *testing.T) {
	if _, ok := summarizeNextBuild("random unrecognized output\n"); ok {
		t.Fatal("expected fallback")
	}
}

func TestNextBuildProcessStderrImmediate(t *testing.T) {
	f := NewNextBuildFilter()
	d := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "fatal\n"}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionImmediate || d.Output != "fatal\n" {
		t.Fatalf("unexpected decision %#v", d)
	}
}
