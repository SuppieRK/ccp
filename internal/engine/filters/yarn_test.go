package filters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestYarnFilterMetadataAndAliases(t *testing.T) {
	f := NewYarnFilter()
	if f.Tool() != "yarn" {
		t.Fatalf("expected yarn tool, got %q", f.Tool())
	}
	wantContains := []string{
		"yarnpkg",
		"yarn.cmd", "./yarn.cmd",
		"yarn.exe", "./yarn.exe",
		"yarnpkg.cmd", "./yarnpkg.cmd",
		"yarnpkg.exe", "./yarnpkg.exe",
	}
	assertAliasesContainFold(t, f.Aliases(), wantContains)
}

func TestYarnPrepareAlwaysForcesPassthrough(t *testing.T) {
	f := NewYarnFilter()
	args := []string{"install", "--immutable"}
	prep := f.Prepare(args)
	if prep.ForcePassthrough {
		t.Fatalf("did not expect force passthrough, got %#v", prep)
	}
	if prep.DispatchKey != "yarn|mode=passthrough" {
		t.Fatalf("unexpected dispatch key: %q", prep.DispatchKey)
	}
	if !slices.Equal(prep.NormalizedArgs, args) {
		t.Fatalf("expected args preserved, want=%#v got=%#v", args, prep.NormalizedArgs)
	}
}

func TestYarnPrepareNoArgsForcesPassthrough(t *testing.T) {
	f := NewYarnFilter()
	prep := f.Prepare(nil)
	if !prep.ForcePassthrough {
		t.Fatalf("expected force passthrough for no-args invocation, got %#v", prep)
	}
	if prep.DispatchKey != "" {
		t.Fatalf("expected empty dispatch key for no-args invocation, got %#v", prep)
	}
	if prep.NormalizedArgs != nil {
		t.Fatalf("expected preserved nil args, got %#v", prep.NormalizedArgs)
	}
}

func TestYarnPrepareRunModeCaseInsensitive(t *testing.T) {
	f := NewYarnFilter()
	args := []string{"RUN", "build"}
	prep := f.Prepare(args)
	if prep.DispatchKey != "yarn|mode=run" {
		t.Fatalf("expected run-mode dispatch, got %#v", prep)
	}
	if !slices.Equal(prep.NormalizedArgs, args) {
		t.Fatalf("expected args preserved, want=%#v got=%#v", args, prep.NormalizedArgs)
	}
}

func TestYarnContextKeySharedAcrossStreams(t *testing.T) {
	f := NewYarnFilter()
	k1 := f.ContextKey(engine.Event{CommandID: "yarn run test", Tool: "yarn", Stream: engine.StdoutStream})
	k2 := f.ContextKey(engine.Event{CommandID: "yarn run test", Tool: "yarn", Stream: engine.StderrStream})
	if k1 != k2 {
		t.Fatalf("expected shared context key across streams, got %q != %q", k1, k2)
	}
}

func TestYarnProcessCollectsPreExitEvents(t *testing.T) {
	f := NewYarnFilter()
	mem := engine.NewOrderedSetBuffer()
	for _, tc := range []engine.EventType{engine.EventLine, engine.EventTick, engine.EventEOF} {
		d := f.Process(engine.Event{Type: tc, Dispatch: "yarn|mode=run", Stream: engine.StdoutStream}, mem)
		if d.Action != engine.ActionCollect {
			t.Fatalf("expected collect for %v, got %#v", tc, d)
		}
	}
}

func TestYarnProcessCompactsRunOutput(t *testing.T) {
	f := NewYarnFilter()
	mem := engine.NewOrderedSetBuffer()
	for i, line := range []string{
		"yarn run v1.22.22\n",
		"$ node scripts/success.js\n",
		"success-line-1\n",
		"Done in 0.06s.\n",
	} {
		_ = mem.Add(line, line, uint64(i+1))
	}
	d := f.Process(engine.Event{Type: engine.EventExit, Dispatch: "yarn|mode=run", ExitCode: 0}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush on exit, got %#v", d)
	}
	if strings.Contains(d.Output, "yarn run v1.22.22") || strings.Contains(d.Output, "Done in 0.06s.") {
		t.Fatalf("expected yarn boilerplate to be removed, got %q", d.Output)
	}
	if !strings.Contains(d.Output, "success-line-1") {
		t.Fatalf("expected meaningful output retained, got %q", d.Output)
	}
}

func TestYarnProcessPassthroughForNonRunMode(t *testing.T) {
	f := NewYarnFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := "success-line-1\n"
	_ = mem.Add(raw, raw, 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Dispatch: "yarn|mode=passthrough", ExitCode: 0}, mem)
	if d.Action != engine.ActionFlush || d.Output != raw {
		t.Fatalf("expected passthrough for non-run mode, got %#v", d)
	}
}

func TestYarnProcessEmptyOutputOnExitUsesParity(t *testing.T) {
	f := NewYarnFilter()
	mem := engine.NewOrderedSetBuffer()
	tests := []struct {
		name     string
		exitCode int
		action   engine.Action
		output   string
		trimmed  bool
	}{
		{name: "success emits ok", exitCode: 0, action: engine.ActionFlush, output: "ok", trimmed: true},
		{name: "failure ignores", exitCode: 1, action: engine.ActionIgnore, output: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := f.Process(engine.Event{Type: engine.EventExit, Dispatch: "yarn|mode=run", ExitCode: tt.exitCode}, mem)
			if d.Action != tt.action {
				t.Fatalf("unexpected action for exit=%d: %#v", tt.exitCode, d)
			}
			if tt.trimmed {
				if strings.TrimSpace(d.Output) != tt.output {
					t.Fatalf("unexpected output for exit=%d: %#v", tt.exitCode, d)
				}
				return
			}
			if d.Output != tt.output {
				t.Fatalf("unexpected output for exit=%d: %#v", tt.exitCode, d)
			}
		})
	}
}

func TestYarnProcessLowConfidenceFallbackFlushesRaw(t *testing.T) {
	f := NewYarnFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := "abc\x00def\n"
	_ = mem.Add(raw, raw, 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Dispatch: "yarn|mode=run", ExitCode: 0}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush on fallback, got %#v", d)
	}
	if d.Output != raw {
		t.Fatalf("expected raw fallback output, want=%q got=%q", raw, d.Output)
	}
}

func TestYarnProcessEmptyCompactedNonZeroFallsBackToRaw(t *testing.T) {
	f := NewYarnFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := strings.Join([]string{
		"yarn run v1.22.22",
		"$ node scripts/fail.js",
		"Done in 0.06s.",
	}, "\n") + "\n"
	_ = mem.Add(raw, raw, 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Dispatch: "yarn|mode=run", ExitCode: 1}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush for non-zero fallback, got %#v", d)
	}
	if d.Output != raw {
		t.Fatalf("expected raw output fallback for empty compacted non-zero result, want=%q got=%q", raw, d.Output)
	}
}
