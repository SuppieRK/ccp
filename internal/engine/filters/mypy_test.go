package filters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestMypyPrepareCases(t *testing.T) {
	f := NewMypyFilter()
	cases := []struct {
		name             string
		args             []string
		forcePassthrough bool
		dispatchKey      string
	}{
		{
			name:        "default-dispatch",
			args:        []string{"src"},
			dispatchKey: mypyDispatchKey,
		},
		{
			name:             "structured-passthrough",
			args:             []string{"--output=json", "src"},
			forcePassthrough: true,
		},
		{
			name:             "report-passthrough",
			args:             []string{"--html-report", "out", "src"},
			forcePassthrough: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if prep.ForcePassthrough != tc.forcePassthrough {
				t.Fatalf("unexpected passthrough: got=%v want=%v", prep.ForcePassthrough, tc.forcePassthrough)
			}
			if prep.DispatchKey != tc.dispatchKey {
				t.Fatalf("unexpected dispatch: got=%q want=%q", prep.DispatchKey, tc.dispatchKey)
			}
		})
	}
}

func TestMypyProcessStderrImmediate(t *testing.T) {
	f := NewMypyFilter()
	d := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "config error\n"}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionImmediate || d.Output != "config error\n" {
		t.Fatalf("expected immediate stderr, got %#v", d)
	}
}

func TestCompactMypyOutput(t *testing.T) {
	raw := strings.Join([]string{
		"src/app.py:12: error: Incompatible return value type (got \"str\", expected \"int\")  [return-value]",
		"src/app.py:13: note: Consider using Optional[str]",
		"src/app.py:18: error: Argument 1 has incompatible type \"int\"; expected \"str\"  [arg-type]",
		"src/models.py:8: error: Name \"foo\" is not defined  [name-defined]",
		"Found 3 errors in 2 files (checked 4 source files)",
	}, "\n")
	out, ok := compactMypyOutput(raw)
	if !ok {
		t.Fatalf("expected summary")
	}
	for _, want := range []string{
		"mypy: 3 errors in 2 files",
		"- src/app.py (2 errors)",
		"L12 [return-value] Incompatible return value type",
		"Consider using Optional[str]",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestCompactMypyFilelessError(t *testing.T) {
	raw := "mypy: error: No module named 'nonexistent'\n"
	out, ok := compactMypyOutput(raw)
	if !ok || !strings.Contains(out, "mypy: error: No module named 'nonexistent'") {
		t.Fatalf("unexpected output: ok=%v out=%q", ok, out)
	}
}

func TestCompactMypySuccess(t *testing.T) {
	out, ok := compactMypyOutput("Success: no issues found in 3 source files\n")
	if !ok || out != "mypy: No issues found\n" {
		t.Fatalf("unexpected success summary: ok=%v out=%q", ok, out)
	}
}
