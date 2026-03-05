package filters

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const (
	npmRunDispatchKey     = "npm|mode=run"
	npmLifecycleBuildLine = "> app@1.0.0 build"
	npmErrorCodeLine      = "npm ERR! code 1"
	npmCompactionExpected = "expected compaction"
)

func TestNPMFilterMetadataAndPrepare(t *testing.T) {
	f := NewNPMFilter()
	if f.Tool() != "npm" {
		t.Fatalf("expected npm tool, got %q", f.Tool())
	}
	wantAliases := []string{"npm.cmd", "./npm.cmd", "npm.exe", "./npm.exe"}
	if !slices.Equal(f.Aliases(), wantAliases) {
		t.Fatalf("unexpected aliases: want=%#v got=%#v", wantAliases, f.Aliases())
	}
	prep := f.Prepare([]string{"run", "build"})
	if prep.ForcePassthrough {
		t.Fatal("expected npm run to stay filtered")
	}
	if prep.DispatchKey != npmRunDispatchKey {
		t.Fatalf("unexpected dispatch key: %q", prep.DispatchKey)
	}
	if !slices.Equal(prep.NormalizedArgs, []string{"run", "build"}) {
		t.Fatalf("unexpected args: %#v", prep.NormalizedArgs)
	}

	prepOther := f.Prepare([]string{"install"})
	if !prepOther.ForcePassthrough {
		t.Fatal("expected non-run subcommand passthrough")
	}
	prepEmpty := f.Prepare(nil)
	if !prepEmpty.ForcePassthrough {
		t.Fatal("expected empty args passthrough")
	}
}

func TestNPMFilterSharedContextAcrossStreams(t *testing.T) {
	f := NewNPMFilter()
	stdout := f.ContextKey(engine.Event{CommandID: "npm run build", Tool: "npm", Stream: engine.StdoutStream})
	stderr := f.ContextKey(engine.Event{CommandID: "npm run build", Tool: "npm", Stream: engine.StderrStream})
	if stdout != stderr {
		t.Fatalf("expected shared context key, got %q != %q", stdout, stderr)
	}
}

func TestClassifyNPMLineClasses(t *testing.T) {
	tests := []struct {
		line    string
		rawLine string
		want    npmOutputClass
	}{
		{line: npmLifecycleBuildLine, rawLine: npmLifecycleBuildLine, want: npmClassLifecycle},
		{line: "npm notice New major version", rawLine: "npm notice New major version", want: npmClassProgress},
		{line: "npm WARN deprecated foo", rawLine: "npm WARN deprecated foo", want: npmClassWarning},
		{line: npmErrorCodeLine, rawLine: npmErrorCodeLine, want: npmClassFailure},
		{line: "⠋ building", rawLine: "\r⠋ building", want: npmClassProgress},
		{line: "PASS test", rawLine: "PASS test", want: npmClassNeutral},
	}
	for _, tc := range tests {
		got := classifyNPMLine(tc.rawLine, tc.line)
		if got != tc.want {
			t.Fatalf("line=%q want=%d got=%d", tc.line, tc.want, got)
		}
	}
}

func TestCompactNPMOutputSuppressesLifecycleAndProgress(t *testing.T) {
	raw := strings.Join([]string{
		npmLifecycleBuildLine,
		"> next build",
		"\r⠋ building...",
		"Build completed",
	}, "\n") + "\n"
	out, ok := compactNPMOutput(raw, npmDispatch{mode: "run"}, 0)
	if !ok {
		t.Fatal(npmCompactionExpected)
	}
	if strings.Contains(out, "> app@") || strings.Contains(out, "⠋") {
		t.Fatalf("expected lifecycle/progress suppressed, got %q", out)
	}
	if !strings.Contains(out, "Build completed") {
		t.Fatalf("expected payload retained, got %q", out)
	}
}

func TestCompactNPMOutputRetainsActionableWarnings(t *testing.T) {
	raw := strings.Join([]string{
		"> app@1.0.0 test",
		"npm WARN deprecated inflight@1.0.6: This module is not supported",
		"Test suite done",
	}, "\n") + "\n"
	out, ok := compactNPMOutput(raw, npmDispatch{mode: "run"}, 0)
	if !ok {
		t.Fatal(npmCompactionExpected)
	}
	if !strings.Contains(out, "npm WARN deprecated") {
		t.Fatalf("expected warning retained, got %q", out)
	}
}

func TestCompactNPMOutputRetainsFailureAndDedupesFooter(t *testing.T) {
	raw := strings.Join([]string{
		npmErrorCodeLine,
		"npm ERR! path /repo",
		"npm ERR! A complete log of this run can be found in:",
		"npm ERR!     /home/user/.npm/_logs/2026-01-01-debug.log",
		"npm ERR! A complete log of this run can be found in:",
		"npm ERR!     /home/user/.npm/_logs/2026-01-01-debug.log",
	}, "\n") + "\n"
	out, ok := compactNPMOutput(raw, npmDispatch{mode: "run"}, 1)
	if !ok {
		t.Fatal(npmCompactionExpected)
	}
	if strings.Count(out, "A complete log of this run can be found in") != 1 {
		t.Fatalf("expected deduped footer pointer, got %q", out)
	}
	if !strings.Contains(out, npmErrorCodeLine) {
		t.Fatalf("expected failure marker retained, got %q", out)
	}
}

func TestCompactNPMOutputOkMarkerOnlyWhenEmptySuccess(t *testing.T) {
	raw := strings.Join([]string{
		npmLifecycleBuildLine,
		"> next build",
		"\r⠋ building...",
	}, "\n") + "\n"
	out, ok := compactNPMOutput(raw, npmDispatch{mode: "run"}, 0)
	if !ok {
		t.Fatal(npmCompactionExpected)
	}
	if strings.TrimSpace(out) != "ok" {
		t.Fatalf("expected ok marker, got %q", out)
	}
}

func TestCompactNPMOutputNoOkWhenMeaningfulOutputExists(t *testing.T) {
	raw := strings.Join([]string{
		npmLifecycleBuildLine,
		"1 file checked",
	}, "\n") + "\n"
	out, ok := compactNPMOutput(raw, npmDispatch{mode: "run"}, 0)
	if !ok {
		t.Fatal(npmCompactionExpected)
	}
	if strings.Contains(out, "ok") {
		t.Fatalf("expected no ok marker with meaningful output, got %q", out)
	}
}

func TestCompactNPMOutputLowConfidenceFallback(t *testing.T) {
	raw := "abc\x00def\n"
	out, ok := compactNPMOutput(raw, npmDispatch{mode: "run"}, 0)
	if ok {
		t.Fatalf("expected fallback, got ok=true output=%q", out)
	}
	if out != raw {
		t.Fatalf("expected raw passthrough, got %q", out)
	}
}

func TestNPMFilterExitFlushUsesExitCode(t *testing.T) {
	f := NewNPMFilter()
	mem := engine.NewOrderedSetBuffer()
	lifecycleBuildLine := npmLifecycleBuildLine + "\n"
	_ = mem.Add(lifecycleBuildLine, lifecycleBuildLine, 1)

	if got := f.Process(engine.Event{Type: engine.EventEOF, Tool: "npm", Dispatch: npmRunDispatchKey}, mem); got.Action != engine.ActionCollect {
		t.Fatalf("expected eof collect, got %q", got.Action)
	}
	got := f.Process(engine.Event{Type: engine.EventExit, Tool: "npm", Dispatch: npmRunDispatchKey, ExitCode: 0}, mem)
	if got.Action != engine.ActionFlush {
		t.Fatalf("expected exit flush, got %q", got.Action)
	}
	if strings.TrimSpace(got.Output) != "ok" {
		t.Fatalf("expected ok marker, got %q", got.Output)
	}
}

func TestNPMFilterCollectsLineAndTickPreExit(t *testing.T) {
	f := NewNPMFilter()
	mem := engine.NewOrderedSetBuffer()
	for _, ev := range []engine.Event{
		{Type: engine.EventLine, Tool: "npm", Dispatch: npmRunDispatchKey, Stream: engine.StdoutStream, Line: "payload\n"},
		{Type: engine.EventTick, Tool: "npm", Dispatch: npmRunDispatchKey, Stream: engine.StdoutStream},
	} {
		got := f.Process(ev, mem)
		if got.Action != engine.ActionCollect {
			t.Fatalf("expected collect for event=%v, got %#v", ev.Type, got)
		}
	}
}

func TestNPMFilterExitFallbackCases(t *testing.T) {
	f := NewNPMFilter()
	cases := []struct {
		name       string
		exitCode   int
		raw        string
		wantAction engine.Action
		wantOutput string
	}{
		{
			name:       "empty-nonzero-ignores",
			exitCode:   1,
			raw:        "",
			wantAction: engine.ActionIgnore,
			wantOutput: "",
		},
		{
			name:       "low-confidence-flushes-raw",
			exitCode:   0,
			raw:        "abc\x00def\n",
			wantAction: engine.ActionFlush,
			wantOutput: "abc\x00def\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			if tc.raw != "" {
				_ = mem.Add(tc.raw, tc.raw, 1)
			}
			got := f.Process(engine.Event{
				Type:     engine.EventExit,
				Tool:     "npm",
				Dispatch: npmRunDispatchKey,
				ExitCode: tc.exitCode,
			}, mem)
			if got.Action != tc.wantAction || got.Output != tc.wantOutput {
				t.Fatalf("unexpected decision: got %#v", got)
			}
		})
	}
}

func TestCompactNPMOutputWarningsRetainFirstDedup(t *testing.T) {
	raw := strings.Join([]string{
		"npm WARN deprecated foo",
		"npm WARN deprecated foo",
		"npm WARN deprecated bar",
	}, "\n") + "\n"
	out, ok := compactNPMOutput(raw, npmDispatch{mode: "run"}, 0)
	if !ok {
		t.Fatal(npmCompactionExpected)
	}
	if strings.Count(out, "npm WARN deprecated foo") != 1 {
		t.Fatalf("expected retain-first dedupe for duplicate warning, got %q", out)
	}
	if !strings.Contains(out, "npm WARN deprecated bar") {
		t.Fatalf("expected distinct warning retained, got %q", out)
	}
}

func TestNPMCompactionThresholdOnNoisyFixture(t *testing.T) {
	var lines []string
	lines = append(lines, "> app@1.0.0 test")
	for i := 0; i < 30; i++ {
		lines = append(lines, "\r⠋ progress "+strconv.Itoa(i))
	}
	lines = append(lines, "PASS sample test")
	raw := strings.Join(lines, "\n") + "\n"
	out, ok := compactNPMOutput(raw, npmDispatch{mode: "run"}, 0)
	if !ok {
		t.Fatal(npmCompactionExpected)
	}
	rawLines := countNonEmptyLines(raw)
	outLines := countNonEmptyLines(out)
	drop := float64(rawLines-outLines) / float64(rawLines)
	if drop < 0.15 {
		t.Fatalf("expected drop >= 0.15, got %.2f (raw=%d out=%d)", drop, rawLines, outLines)
	}
}
