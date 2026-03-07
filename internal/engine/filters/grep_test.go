package filters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const grepExpectedCompactSuccess = "expected compact success"

func TestGrepPrepareNormalizesSubstitutionAndDefaultPath(t *testing.T) {
	f := NewGrepFilter()
	prep := f.Prepare([]string{"foo"})
	if prep.PreferredSubstitution != "rg" {
		t.Fatalf("expected preferred substitution rg, got %q", prep.PreferredSubstitution)
	}
	if len(prep.PreferredArgs) == 0 || prep.PreferredArgs[len(prep.PreferredArgs)-1] != "." {
		t.Fatalf("expected default path '.', got %#v", prep.PreferredArgs)
	}
	if !containsArg(prep.PreferredArgs, "--color=never") {
		t.Fatalf("expected deterministic output flags, got %#v", prep.PreferredArgs)
	}
	if !containsArg(prep.FallbackArgs, "-r") {
		t.Fatalf("expected recursive fallback args, got %#v", prep.FallbackArgs)
	}
}

func TestGrepMetadataAndAliases(t *testing.T) {
	f := NewGrepFilter()
	if f.Tool() != "grep" {
		t.Fatalf("expected grep tool, got %q", f.Tool())
	}
	if !containsArg(f.Aliases(), "rg") {
		t.Fatalf("expected rg alias, got %#v", f.Aliases())
	}
}

func TestGrepPrepareTranslatesBREAlternation(t *testing.T) {
	f := NewGrepFilter()
	prep := f.Prepare([]string{`a\|b`, "."})
	if !containsArg(prep.PreferredArgs, "a|b") {
		t.Fatalf("expected translated pattern in preferred args, got %#v", prep.PreferredArgs)
	}
}

func TestGrepPrepareRecursiveHandlingByBackend(t *testing.T) {
	f := NewGrepFilter()
	prep := f.Prepare([]string{"-r", "needle", "."})
	if containsArg(prep.PreferredArgs, "-r") || containsArg(prep.PreferredArgs, "--recursive") {
		t.Fatalf("did not expect recursive flags in rg preferred args, got %#v", prep.PreferredArgs)
	}
	if !containsArg(prep.FallbackArgs, "-r") {
		t.Fatalf("expected recursive flag preserved in grep fallback args, got %#v", prep.FallbackArgs)
	}
}

func TestGrepPrepareUnsafeRegexForcesAmbiguity(t *testing.T) {
	f := NewGrepFilter()
	prep := f.Prepare([]string{`a\+b`, "."})
	if !prep.Ambiguous || !prep.ForcePassthrough {
		t.Fatalf("expected ambiguous passthrough for unsafe translation, got %#v", prep)
	}
}

func TestCompactGrepOutputGroupedAndCapped(t *testing.T) {
	raw := strings.Join([]string{
		"src/a.go:10:alpha",
		"src/a.go:11:beta",
		"src/b.go:7:gamma",
	}, "\n") + "\n"
	out, ok := compactGrepOutput(raw, grepDispatch{maxResults: 2})
	if !ok {
		t.Fatal(grepExpectedCompactSuccess)
	}
	if !strings.Contains(out, "src/a.go (2 matches)") {
		t.Fatalf("expected grouped output, got %q", out)
	}
	if !strings.Contains(out, "... +1 more matches") {
		t.Fatalf("expected remaining indicator, got %q", out)
	}
}

func TestCompactGrepOutputContextOnly(t *testing.T) {
	long := "src/a.go:10:" + strings.Repeat("x", 200)
	out, ok := compactGrepOutput(long+"\n", grepDispatch{maxResults: 10, contextOnly: true})
	if !ok {
		t.Fatal(grepExpectedCompactSuccess)
	}
	if len(out) == 0 || strings.Count(out, "x") >= 200 {
		t.Fatalf("expected truncated context-only text, got %q", out)
	}
}

func TestGrepFilterNoMatchCases(t *testing.T) {
	f := NewGrepFilter()
	cases := []struct {
		name          string
		dispatch      string
		exitCode      int
		runEOFFirst   bool
		wantEOFAction engine.Action
		wantAction    engine.Action
		wantOutput    string
	}{
		{
			name:          "non-zero-no-match-after-eof",
			dispatch:      "grep|max=50|context_only=0",
			exitCode:      1,
			runEOFFirst:   true,
			wantEOFAction: engine.ActionCollect,
			wantAction:    engine.ActionIgnore,
		},
		{
			name:       "zero-exit-no-match-empty",
			dispatch:   "grep|max=50|context_only=0",
			exitCode:   0,
			wantAction: engine.ActionIgnore,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			if tc.runEOFFirst {
				eof := f.Process(engine.Event{
					Type:     engine.EventEOF,
					Tool:     "grep",
					Dispatch: tc.dispatch,
					Stream:   engine.StdoutStream,
				}, mem)
				if eof.Action != tc.wantEOFAction {
					t.Fatalf("unexpected EOF action: got %v want %v", eof.Action, tc.wantEOFAction)
				}
			}

			exit := f.Process(engine.Event{
				Type:     engine.EventExit,
				Tool:     "grep",
				Dispatch: tc.dispatch,
				Stream:   engine.StdoutStream,
				ExitCode: tc.exitCode,
			}, mem)
			if exit.Action != tc.wantAction || exit.Output != tc.wantOutput {
				t.Fatalf("unexpected exit decision: got %#v", exit)
			}
		})
	}
}

func TestGrepFilterStderrImmediateAndNormalized(t *testing.T) {
	f := NewGrepFilter()
	mem := engine.NewOrderedSetBuffer()
	ev := engine.Event{
		Type:   engine.EventLine,
		Tool:   "grep",
		Stream: engine.StderrStream,
		Line:   "rg: ./missing: IO error for operation on ./missing: No such file or directory (os error 2)\n",
	}
	d := f.Process(ev, mem)
	if d.Action != engine.ActionImmediate {
		t.Fatalf("expected immediate stderr handling, got %#v", d)
	}
	if d.Output != "grep: ./missing: No such file or directory\n" {
		t.Fatalf("unexpected normalized stderr line: %q", d.Output)
	}
}

func TestGrepFilterCollectsStdoutLineTickAndEOF(t *testing.T) {
	f := NewGrepFilter()
	mem := engine.NewOrderedSetBuffer()
	for _, ev := range []engine.Event{
		{Type: engine.EventLine, Tool: "grep", Stream: engine.StdoutStream, Line: "a.go:1:x\n"},
		{Type: engine.EventTick, Tool: "grep", Stream: engine.StdoutStream},
		{Type: engine.EventEOF, Tool: "grep", Stream: engine.StdoutStream},
	} {
		d := f.Process(ev, mem)
		if d.Action != engine.ActionCollect {
			t.Fatalf("expected collect for event=%v, got %#v", ev.Type, d)
		}
	}
}

func TestGrepFilterParseFailureFallsBackToRaw(t *testing.T) {
	f := NewGrepFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := "unparseable line\n"
	_ = mem.Add(raw, "k", 1)
	d := f.Process(engine.Event{
		Type:     engine.EventExit,
		Tool:     "grep",
		Dispatch: "grep|max=50|context_only=0",
		Stream:   engine.StdoutStream,
	}, mem)
	if d.Action != engine.ActionFlush || d.Output != raw {
		t.Fatalf("expected raw fallback flush, got %#v", d)
	}
}

func TestNormalizeGrepErrorLine(t *testing.T) {
	in := "rg: ./does-not-exist: IO error for operation on ./does-not-exist: No such file or directory (os error 2)\n"
	got := normalizeGrepErrorLine(in)
	want := "grep: ./does-not-exist: No such file or directory\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestGrepCompactionThreshold(t *testing.T) {
	lines := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		lines = append(lines, "src/file.go:"+itoa(i+1)+":needle in very long line content for threshold validation")
	}
	raw := strings.Join(lines, "\n") + "\n"
	out, ok := compactGrepOutput(raw, grepDispatch{maxResults: 10})
	if !ok {
		t.Fatal(grepExpectedCompactSuccess)
	}
	rawLines := countNonEmptyLines(raw)
	outLines := countNonEmptyLines(out)
	dropRatio := float64(rawLines-outLines) / float64(rawLines)
	if dropRatio < 0.15 {
		t.Fatalf("expected drop ratio >= 0.15, got %.2f (raw=%d out=%d)", dropRatio, rawLines, outLines)
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 12)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
