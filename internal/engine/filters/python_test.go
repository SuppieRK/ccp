package filters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestPythonFilterMetadataAndAliases(t *testing.T) {
	f := NewPythonFilter()
	if f.Tool() != "python" {
		t.Fatalf("expected python tool, got %q", f.Tool())
	}
	wantContains := []string{
		"python3",
		"python.exe", "./python.exe",
		"python3.exe", "./python3.exe",
		"python.cmd", "./python.cmd",
		"python3.cmd", "./python3.cmd",
	}
	assertAliasesContainFold(t, f.Aliases(), wantContains)
}

func TestPythonPreparePassthroughPreservesArgs(t *testing.T) {
	f := NewPythonFilter()
	args := []string{"script.py", "--flag"}
	prep := f.Prepare(args)
	if !prep.ForcePassthrough {
		t.Fatalf("expected passthrough prepare, got %#v", prep)
	}
	if prep.Ambiguous {
		t.Fatalf("did not expect ambiguous script invocation, got %#v", prep)
	}
	if !slices.Equal(prep.NormalizedArgs, args) {
		t.Fatalf("expected args preserved, want=%#v got=%#v", args, prep.NormalizedArgs)
	}
}

func TestPythonPrepareInteractiveAmbiguousPassthrough(t *testing.T) {
	f := NewPythonFilter()
	tests := [][]string{nil, {}, {"-i"}, {"--interactive"}, {"-I", "-i"}}
	for _, tc := range tests {
		prep := f.Prepare(tc)
		if !prep.ForcePassthrough || !prep.Ambiguous {
			t.Fatalf("expected ambiguous passthrough for args %#v, got %#v", tc, prep)
		}
		if prep.Reason != "interactive python invocation" {
			t.Fatalf("expected interactive reason for args %#v, got %#v", tc, prep)
		}
	}
}

func TestPythonPrepareModulePytestRouting(t *testing.T) {
	f := NewPythonFilter()
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "lowercase",
			args: []string{"-m", "pytest", "-q"},
			want: []string{"-m", "pytest", "-q", "--tb=short", "--no-header"},
		},
		{
			name: "case-insensitive",
			args: []string{"-m", "PyTeSt", "-q"},
			want: []string{"-m", "PyTeSt", "-q", "--tb=short", "--no-header"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prep := f.Prepare(tt.args)
			if prep.ForcePassthrough {
				t.Fatalf("expected pytest module invocation to be routed, got %#v", prep)
			}
			if prep.DispatchKey != "pytest" {
				t.Fatalf("expected pytest dispatch key, got %#v", prep)
			}
			if !slices.Equal(prep.NormalizedArgs, tt.want) {
				t.Fatalf("expected delegated pytest args, want=%#v got=%#v", tt.want, prep.NormalizedArgs)
			}
		})
	}
}

func TestPythonPrepareModuleNonPytestPassthrough(t *testing.T) {
	f := NewPythonFilter()
	args := []string{"-m", "pip", "list"}
	prep := f.Prepare(args)
	if !prep.ForcePassthrough {
		t.Fatalf("expected passthrough for non-pytest module invocation, got %#v", prep)
	}
	if prep.DispatchKey != "" {
		t.Fatalf("expected no dispatch for non-pytest module invocation, got %#v", prep)
	}
}

func TestPythonContextKeyIsolationByCommand(t *testing.T) {
	f := NewPythonFilter()
	k1 := f.ContextKey(engine.Event{CommandID: "python app.py", Tool: "python", Stream: engine.StdoutStream})
	k2 := f.ContextKey(engine.Event{CommandID: "npm run test", Tool: "python", Stream: engine.StdoutStream})
	if k1 == k2 {
		t.Fatalf("expected distinct context keys for different command IDs, got %q", k1)
	}
}

func TestPythonContextKeyDelegatesPytestDispatch(t *testing.T) {
	f := NewPythonFilter()
	ev := engine.Event{
		CommandID: "python -m pytest",
		Tool:      "python",
		Dispatch:  "pytest",
		Stream:    engine.StdoutStream,
	}
	got := f.ContextKey(ev)
	want := NewPytestFilter().ContextKey(ev)
	if got != want {
		t.Fatalf("expected delegated context key, want %q got %q", want, got)
	}
}

func TestPythonProcessPassthroughNoSyntheticMarker(t *testing.T) {
	f := NewPythonFilter()
	tests := []struct {
		name string
		ev   engine.Event
		want engine.Decision
	}{
		{
			name: "line immediate passthrough",
			ev:   engine.Event{Type: engine.EventLine, Line: "hello\n"},
			want: engine.Decision{Action: engine.ActionImmediate, Output: "hello\n"},
		},
		{
			name: "tick ignored",
			ev:   engine.Event{Type: engine.EventTick},
			want: engine.Decision{Action: engine.ActionIgnore},
		},
		{
			name: "eof ignored",
			ev:   engine.Event{Type: engine.EventEOF},
			want: engine.Decision{Action: engine.ActionIgnore},
		},
		{
			name: "exit ignored",
			ev:   engine.Event{Type: engine.EventExit, ExitCode: 0},
			want: engine.Decision{Action: engine.ActionIgnore},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Process(tt.ev, engine.NewOrderedSetBuffer())
			if got.Action != tt.want.Action || got.Output != tt.want.Output {
				t.Fatalf("unexpected decision, want=%#v got=%#v", tt.want, got)
			}
		})
	}
}

func TestPythonProcessDelegatesPytestDispatch(t *testing.T) {
	f := NewPythonFilter()
	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add("=== 1 passed in 0.01s ===\n", "=== 1 passed in 0.01s ===\n", 1)
	d := f.Process(engine.Event{
		Type:     engine.EventExit,
		Tool:     "python",
		Dispatch: "pytest",
		Stream:   engine.StdoutStream,
	}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush decision for pytest dispatch, got %#v", d)
	}
	if !strings.Contains(d.Output, "pytest: 1 passed") {
		t.Fatalf("expected pytest compact output, got %#v", d)
	}
}
