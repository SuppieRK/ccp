package filters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const (
	denoRunDispatch     = "deno|mode=run"
	denoDownloadLine    = "Download https://deno.land/std/mod.ts"
	denoPromptLine      = "Allow read access? [y/n]\n"
	denoExpectedCompact = "expected compaction"
)

func TestDenoFilterMetadataAndPrepare(t *testing.T) {
	f := NewDenoFilter()
	if f.Tool() != "deno" {
		t.Fatalf("expected deno tool, got %q", f.Tool())
	}
	wantAliases := []string{"deno.exe", "./deno.exe", "deno.cmd", "./deno.cmd"}
	if !slices.Equal(f.Aliases(), wantAliases) {
		t.Fatalf("unexpected aliases: want=%#v got=%#v", wantAliases, f.Aliases())
	}

	prep := f.Prepare([]string{"run", "app.ts"})
	if prep.ForcePassthrough {
		t.Fatal("expected run mode to stay filtered")
	}
	if prep.DispatchKey != denoRunDispatch {
		t.Fatalf("unexpected dispatch key: %q", prep.DispatchKey)
	}
}

func TestDenoPrepareSubcommandModes(t *testing.T) {
	f := NewDenoFilter()
	tests := []struct {
		args            []string
		wantDispatch    string
		wantPassthrough bool
	}{
		{args: []string{"run", "a.ts"}, wantDispatch: denoRunDispatch},
		{args: []string{"test"}, wantDispatch: "deno|mode=test"},
		{args: []string{"lint"}, wantDispatch: "deno|mode=lint"},
		{args: []string{"check"}, wantDispatch: "deno|mode=check"},
		{args: []string{"fmt"}, wantDispatch: "deno|mode=lint"},
		{args: []string{"repl"}, wantDispatch: "deno|mode=repl", wantPassthrough: true},
		{args: []string{"task", "build"}, wantDispatch: "deno|mode=task", wantPassthrough: true},
		{args: []string{"unknown"}, wantDispatch: "deno|mode=unknown", wantPassthrough: true},
	}
	for _, tc := range tests {
		prep := f.Prepare(tc.args)
		if prep.DispatchKey != tc.wantDispatch {
			t.Fatalf("args=%#v want dispatch=%q got=%q", tc.args, tc.wantDispatch, prep.DispatchKey)
		}
		if prep.ForcePassthrough != tc.wantPassthrough {
			t.Fatalf("args=%#v want passthrough=%v got=%v", tc.args, tc.wantPassthrough, prep.ForcePassthrough)
		}
	}
}

func TestDenoPrepareStructuredOutputPassthrough(t *testing.T) {
	f := NewDenoFilter()
	cases := []struct {
		name         string
		args         []string
		wantDispatch string
	}{
		{name: "json-flag", args: []string{"test", "--json"}, wantDispatch: "deno|mode=structured"},
		{name: "json-option", args: []string{"lint", "--json=pretty", "lint_target.ts"}, wantDispatch: "deno|mode=structured"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if !prep.ForcePassthrough {
				t.Fatalf("expected structured output args to force passthrough for %#v", tc.args)
			}
			if !prep.Ambiguous {
				t.Fatalf("expected structured output args to mark ambiguous for %#v", tc.args)
			}
			if prep.DispatchKey != tc.wantDispatch {
				t.Fatalf("unexpected structured dispatch key: got %q want %q", prep.DispatchKey, tc.wantDispatch)
			}
		})
	}
}

func TestDenoFilterSharedContextAcrossStreams(t *testing.T) {
	f := NewDenoFilter()
	stdout := f.ContextKey(engine.Event{CommandID: "deno run app.ts", Tool: "deno", Stream: engine.StdoutStream})
	stderr := f.ContextKey(engine.Event{CommandID: "deno run app.ts", Tool: "deno", Stream: engine.StderrStream})
	if stdout != stderr {
		t.Fatalf("expected shared context key, got %q != %q", stdout, stderr)
	}
}

func TestDenoLifecycleRegexMatchesExpectedPrefixes(t *testing.T) {
	okLines := []string{
		denoDownloadLine,
		"Check file:///repo/main.ts",
		"Compile https://deno.land/x/a.ts",
		"Bundle file:///repo/a.ts",
	}
	for _, line := range okLines {
		if !denoLifecyclePrefixRe.MatchString(line) {
			t.Fatalf("expected lifecycle match for %q", line)
		}
	}
	badLines := []string{
		"download https://deno.land/std/mod.ts",
		"Downloading https://deno.land/std/mod.ts",
		"Check this payload please",
	}
	for _, line := range badLines {
		if denoLifecyclePrefixRe.MatchString(line) {
			t.Fatalf("expected no lifecycle match for %q", line)
		}
	}
}

func TestDenoCompactionFoldsProgressLines(t *testing.T) {
	raw := strings.Join([]string{
		denoDownloadLine,
		"Download https://deno.land/std/other.ts",
		"Check file:///repo/main.ts",
		"Check file:///repo/lib.ts",
		"app payload",
	}, "\n") + "\n"
	out, ok := compactDenoOutput(raw, denoDispatch{mode: "run"}, 0)
	if !ok {
		t.Fatal(denoExpectedCompact)
	}
	if !strings.Contains(out, "[+1 similar progress lines]") {
		t.Fatalf("expected progress fold summary, got %q", out)
	}
	if !strings.Contains(out, "app payload") {
		t.Fatalf("expected payload retained, got %q", out)
	}
}

func TestDenoCompactionRetainsFailuresAndPanicSignals(t *testing.T) {
	raw := strings.Join([]string{
		denoDownloadLine,
		"error: TS2304 Cannot find name x",
		"panic: unrecoverable runtime error",
		"stack backtrace:",
	}, "\n") + "\n"
	out, ok := compactDenoOutput(raw, denoDispatch{mode: "test"}, 1)
	if !ok {
		t.Fatal(denoExpectedCompact)
	}
	if !strings.Contains(out, "error: TS2304") || !strings.Contains(out, "panic:") || !strings.Contains(out, "stack backtrace:") {
		t.Fatalf("expected failure and panic lines retained, got %q", out)
	}
}

func TestDenoTestFailureCompactsTypeErrors(t *testing.T) {
	raw := strings.Join([]string{
		"Check main_test.ts",
		"TS2367 [ERROR]: This comparison appears to be unintentional because the types '1' and '2' have no overlap.",
		"  if (left !== right) {",
		"      ~~~~~~~~~~~~~~",
		"    at file:///repo/main_test.ts:4:7",
		"",
		"error: Type checking failed.",
		"",
		"  info: The program failed type-checking, but it still might work correctly.",
		"  hint: Re-run with --no-check to skip type-checking.",
	}, "\n") + "\n"
	out, ok := compactDenoOutput(raw, denoDispatch{mode: "test"}, 1)
	if !ok {
		t.Fatal(denoExpectedCompact)
	}
	if strings.Contains(out, "Check main_test.ts") {
		t.Fatalf("expected progress line to be removed on failure, got %q", out)
	}
	if !strings.Contains(out, "TS2367 [ERROR] at file:///repo/main_test.ts:4:7") {
		t.Fatalf("expected compacted TS error with location, got %q", out)
	}
	if !strings.Contains(out, "error: Type checking failed.") || !strings.Contains(out, "hint: Re-run with --no-check to skip type-checking.") {
		t.Fatalf("expected actionable failure lines retained, got %q", out)
	}
}

func TestDenoPromptFlushesImmediately(t *testing.T) {
	f := NewDenoFilter()
	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add(denoDownloadLine+"\n", denoDownloadLine+"\n", 1)
	_ = mem.Add(denoPromptLine, denoPromptLine, 2)
	d := f.Process(engine.Event{
		Type:     engine.EventLine,
		Tool:     "deno",
		Dispatch: denoRunDispatch,
		Line:     denoPromptLine,
		Stream:   engine.StdoutStream,
	}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected immediate flush for prompt, got %q", d.Action)
	}
	if !strings.Contains(d.Output, strings.TrimSuffix(denoPromptLine, "\n")) {
		t.Fatalf("expected prompt in output, got %q", d.Output)
	}
}

func TestDenoPromptSingleLineFastPath(t *testing.T) {
	f := NewDenoFilter()
	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add(denoPromptLine, denoPromptLine, 1)
	d := f.Process(engine.Event{
		Type:     engine.EventLine,
		Tool:     "deno",
		Dispatch: denoRunDispatch,
		Line:     denoPromptLine,
		Stream:   engine.StdoutStream,
	}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected immediate flush for prompt, got %q", d.Action)
	}
	if d.Output != denoPromptLine {
		t.Fatalf("expected single-line fast-path flush, got %q", d.Output)
	}
}

func TestDenoPanicFlushesImmediately(t *testing.T) {
	f := NewDenoFilter()
	mem := engine.NewOrderedSetBuffer()
	panicLine := "panic: unrecoverable runtime error\n"
	_ = mem.Add(denoDownloadLine+"\n", denoDownloadLine+"\n", 1)
	_ = mem.Add(panicLine, panicLine, 2)
	d := f.Process(engine.Event{
		Type:     engine.EventLine,
		Tool:     "deno",
		Dispatch: denoRunDispatch,
		Line:     panicLine,
		Stream:   engine.StderrStream,
	}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected immediate flush for panic, got %q", d.Action)
	}
	if !strings.Contains(d.Output, "panic: unrecoverable runtime error") {
		t.Fatalf("expected panic in output, got %q", d.Output)
	}
}

func TestDenoExitReturnsOkMarkerOnlyWhenFullySuppressedSuccess(t *testing.T) {
	f := NewDenoFilter()
	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add(denoDownloadLine+"\n", denoDownloadLine+"\n", 1)
	_ = mem.Add("Check file:///repo/main.ts\n", "Check file:///repo/main.ts\n", 2)
	d := f.Process(engine.Event{Type: engine.EventExit, Tool: "deno", Dispatch: denoRunDispatch, ExitCode: 0}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush, got %q", d.Action)
	}
	if strings.TrimSpace(d.Output) != "ok" {
		t.Fatalf("expected ok marker, got %q", d.Output)
	}
}

func TestDenoTestFailureBlocksOkMarker(t *testing.T) {
	raw := strings.Join([]string{
		denoDownloadLine,
		"error: test failed",
	}, "\n") + "\n"
	out, ok := compactDenoOutput(raw, denoDispatch{mode: "test"}, 0)
	if !ok {
		t.Fatal(denoExpectedCompact)
	}
	if strings.TrimSpace(out) == "ok" {
		t.Fatalf("did not expect ok marker on test failure signal, got %q", out)
	}
}

func TestDenoLowConfidenceFallback(t *testing.T) {
	raw := "abc\x00def\n"
	out, ok := compactDenoOutput(raw, denoDispatch{mode: "run"}, 0)
	if ok {
		t.Fatalf("expected fallback, got ok=true output=%q", out)
	}
	if out != raw {
		t.Fatalf("expected raw passthrough, got %q", out)
	}
}

func TestDenoCheckFailureRetainsDiagnostics(t *testing.T) {
	raw := strings.Join([]string{
		"Check file:///repo/main.ts",
		"error: Type checking failed.",
		"at file:///repo/main.ts:7:3",
	}, "\n") + "\n"
	out, ok := compactDenoOutput(raw, denoDispatch{mode: "check"}, 1)
	if !ok {
		t.Fatal(denoExpectedCompact)
	}
	if strings.Contains(out, "ok\n") {
		t.Fatalf("expected no success marker for failed check, got %q", out)
	}
	if !strings.Contains(out, "error: Type checking failed.") || !strings.Contains(out, "at file:///repo/main.ts:7:3") {
		t.Fatalf("expected diagnostics retained, got %q", out)
	}
}

func TestDenoStderrDiagnosticVisibilityOnExit(t *testing.T) {
	f := NewDenoFilter()
	mem := engine.NewOrderedSetBuffer()
	diag := "error: PermissionDenied: Requires net access\n"
	_ = mem.Add(diag, strings.TrimSpace(diag), 1)
	d := f.Process(engine.Event{
		Type:     engine.EventExit,
		Tool:     "deno",
		Dispatch: denoRunDispatch,
		Stream:   engine.StderrStream,
		ExitCode: 1,
	}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush on exit, got %q", d.Action)
	}
	if !strings.Contains(d.Output, "error: PermissionDenied") {
		t.Fatalf("expected stderr diagnostic retained, got %q", d.Output)
	}
}

func TestDenoCompactionThresholdOnNoisyFixture(t *testing.T) {
	lines := make([]string, 0, 50)
	for i := 0; i < 40; i++ {
		lines = append(lines, denoDownloadLine)
	}
	lines = append(lines, "payload")
	raw := strings.Join(lines, "\n") + "\n"
	out, ok := compactDenoOutput(raw, denoDispatch{mode: "run"}, 0)
	if !ok {
		t.Fatal(denoExpectedCompact)
	}
	rawLines := countNonEmptyLines(raw)
	outLines := countNonEmptyLines(out)
	drop := float64(rawLines-outLines) / float64(rawLines)
	if drop < 0.15 {
		t.Fatalf("expected drop >= 0.15, got %.2f (raw=%d out=%d)", drop, rawLines, outLines)
	}
}
