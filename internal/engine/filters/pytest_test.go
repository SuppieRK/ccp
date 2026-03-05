package filters

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const pytestExpectedFlushMsg = "expected flush, got %#v"

func TestPytestMetadataAndAliases(t *testing.T) {
	f := NewPytestFilter()
	if got := f.Tool(); got != "pytest" {
		t.Fatalf("expected tool pytest, got %q", got)
	}
	wantAliases := []string{"pytest.exe", "./pytest.exe", "pytest.cmd", "./pytest.cmd"}
	if !slices.Equal(f.Aliases(), wantAliases) {
		t.Fatalf("unexpected aliases: want=%#v got=%#v", wantAliases, f.Aliases())
	}
}

func TestPytestPrepareInjectsCompactDefaults(t *testing.T) {
	f := NewPytestFilter()
	prep := f.Prepare([]string{"tests/"})
	want := []string{"tests/", "--tb=short", "--no-header"}
	if !slices.Equal(prep.NormalizedArgs, want) {
		t.Fatalf("unexpected normalized args: want=%#v got=%#v", want, prep.NormalizedArgs)
	}
}

func TestPytestPrepareBacksOffOnUserTracebackOrVerbose(t *testing.T) {
	f := NewPytestFilter()
	cases := [][]string{
		{"--tb=long"},
		{"-vv"},
		{"--verbose"},
	}
	for _, args := range cases {
		prep := f.Prepare(args)
		if !slices.Equal(prep.NormalizedArgs, args) {
			t.Fatalf("expected args preserved for explicit troubleshooting flags, want=%#v got=%#v", args, prep.NormalizedArgs)
		}
	}
}

func TestPytestProcessExitSummariesAndFallback(t *testing.T) {
	f := NewPytestFilter()
	tests := []struct {
		name    string
		raw     string
		want    string
		trimmed bool
	}{
		{
			name:    "pass summary",
			raw:     "=== 3 passed in 0.15s ===\n",
			want:    "pytest: 3 passed",
			trimmed: true,
		},
		{
			name:    "no tests collected",
			raw:     "collected 0 items\n\n=== no tests ran in 0.00s ===\n",
			want:    "pytest: no tests collected",
			trimmed: true,
		},
		{
			name:    "complete summary",
			raw:     "collected 3 items\n",
			want:    "pytest: complete",
			trimmed: true,
		},
		{
			name: "raw fallback",
			raw:  "random non-pytest output\n",
			want: "random non-pytest output\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			_ = mem.Add(tt.raw, tt.raw, 1)
			d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream}, mem)
			if d.Action != engine.ActionFlush {
				t.Fatalf(pytestExpectedFlushMsg, d)
			}
			if tt.trimmed {
				if strings.TrimSpace(d.Output) != tt.want {
					t.Fatalf("unexpected output: %q", d.Output)
				}
				return
			}
			if d.Output != tt.want {
				t.Fatalf("expected raw fallback output, want=%q got=%q", tt.want, d.Output)
			}
		})
	}
}

func TestPytestProcessFailureTopThreeAndFailedList(t *testing.T) {
	f := NewPytestFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := strings.Join([]string{
		"=== FAILURES ===",
		"___ tests/test_app.py::test_one ___",
		"    10 foo",
		">   11 assert 1 == 2",
		"    12 bar",
		"E   AssertionError: assert 1 == 2",
		"----------------------------- Captured stdout call -----------------------------",
		"line from failed test",
		"___ tests/test_app.py::test_two ___",
		">   20 assert False",
		"E   AssertionError: assert False",
		"___ tests/test_app.py::test_three ___",
		">   30 assert 3 == 4",
		"E   AssertionError: assert 3 == 4",
		"___ tests/test_app.py::test_four ___",
		">   40 assert 4 == 5",
		"E   AssertionError: assert 4 == 5",
		"=== short test summary info ===",
		"FAILED tests/test_app.py::test_one - AssertionError: assert 1 == 2",
		"FAILED tests/test_app.py::test_two - AssertionError: assert False",
		"FAILED tests/test_app.py::test_three - AssertionError: assert 3 == 4",
		"FAILED tests/test_app.py::test_four - AssertionError: assert 4 == 5",
		"=== 1 passed, 4 failed in 0.20s ===",
	}, "\n") + "\n"
	_ = mem.Add(raw, raw, 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf(pytestExpectedFlushMsg, d)
	}
	if !strings.Contains(d.Output, "pytest: 1 passed, 4 failed") {
		t.Fatalf("missing failure summary: %q", d.Output)
	}
	if strings.Count(d.Output, "- tests/test_app.py::") < 3 {
		t.Fatalf("expected top three failure details, got %q", d.Output)
	}
	if strings.Contains(d.Output, "- tests/test_app.py::test_four:") {
		t.Fatalf("expected detailed failures to be limited to top 3, got %q", d.Output)
	}
	if !strings.Contains(d.Output, "failed tests:\n- tests/test_app.py::test_one - AssertionError: assert 1 == 2") {
		t.Fatalf("expected full failed tests summary list, got %q", d.Output)
	}
	if !strings.Contains(d.Output, "line from failed test") {
		t.Fatalf("expected captured output for failed test, got %q", d.Output)
	}
}

func TestPytestStderrPassthroughOnly(t *testing.T) {
	f := NewPytestFilter()
	mem := engine.NewOrderedSetBuffer()
	line := "traceback on stderr\n"
	d := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: line}, mem)
	if d.Action != engine.ActionImmediate || d.Output != line {
		t.Fatalf("expected stderr passthrough immediate, got %#v", d)
	}
}

func TestPytestStdoutPreExitEventHandling(t *testing.T) {
	f := NewPytestFilter()
	mem := engine.NewOrderedSetBuffer()
	line := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StdoutStream, Line: "line\n"}, mem)
	if line.Action != engine.ActionCollect {
		t.Fatalf("expected stdout line collect, got %#v", line)
	}
	tick := f.Process(engine.Event{Type: engine.EventTick, Stream: engine.StdoutStream}, mem)
	if tick.Action != engine.ActionCollect {
		t.Fatalf("expected stdout tick collect, got %#v", tick)
	}
	eof := f.Process(engine.Event{Type: engine.EventEOF, Stream: engine.StdoutStream}, mem)
	if eof.Action != engine.ActionIgnore || eof.Output != "" {
		t.Fatalf("expected stdout EOF ignore, got %#v", eof)
	}
}

func TestPytestProcessExitEmptyBufferIgnored(t *testing.T) {
	f := NewPytestFilter()
	mem := engine.NewOrderedSetBuffer()
	d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream}, mem)
	if d.Action != engine.ActionIgnore || d.Output != "" {
		t.Fatalf("expected empty stdout exit to ignore, got %#v", d)
	}
}

func TestPytestFailureContextAndCaptureBounds(t *testing.T) {
	f := NewPytestFilter()
	mem := engine.NewOrderedSetBuffer()
	lines := []string{
		"=== FAILURES ===",
		"___ tests/test_bounds.py::test_context_capture ___",
	}
	for i := 1; i <= 12; i++ {
		if i == 8 {
			lines = append(lines, ">   "+strconv.Itoa(i)+" boom_line_"+strconv.Itoa(i))
			continue
		}
		lines = append(lines, "    "+strconv.Itoa(i)+" ctx_line_"+strconv.Itoa(i))
	}
	lines = append(lines,
		"E   AssertionError: bounded",
		"----------------------------- Captured stdout call -----------------------------",
	)
	for i := 1; i <= 20; i++ {
		lines = append(lines, "cap_line_"+strconv.Itoa(i))
	}
	lines = append(lines,
		"=== short test summary info ===",
		"FAILED tests/test_bounds.py::test_context_capture - AssertionError: bounded",
		"=== 0 passed, 1 failed in 0.01s ===",
	)
	raw := strings.Join(lines, "\n") + "\n"
	_ = mem.Add(raw, raw, 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf(pytestExpectedFlushMsg, d)
	}
	// Context is bounded to 3 lines before and 3 lines after the failing source marker.
	if strings.Contains(d.Output, "ctx_line_4") || strings.Contains(d.Output, "ctx_line_12") {
		t.Fatalf("expected bounded context window, got %q", d.Output)
	}
	for _, want := range []string{"ctx_line_5", "ctx_line_6", "ctx_line_7", "boom_line_8", "ctx_line_9", "ctx_line_10", "ctx_line_11"} {
		if !strings.Contains(d.Output, want) {
			t.Fatalf("missing expected context line %q in %q", want, d.Output)
		}
	}
	// Captured output retains at most 12 lines after the capture header.
	if !strings.Contains(d.Output, "cap_line_12") {
		t.Fatalf("expected retained capture line in %q", d.Output)
	}
	if strings.Contains(d.Output, "cap_line_13") {
		t.Fatalf("expected capture lines beyond budget to be dropped, got %q", d.Output)
	}
}

func TestPytestLowConfidenceFallback(t *testing.T) {
	if _, ok := compactPytestOutput("abc\x00def\n"); ok {
		t.Fatal("expected low-confidence fallback")
	}
}
