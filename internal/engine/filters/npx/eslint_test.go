package npxfilters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const eslintLintFailPath = "src/lint_fail.js"

func TestNpxEslintPrepareCases(t *testing.T) {
	f := NewNpxEslintFilter()
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "injects-json-format",
			args: []string{eslintLintFailPath},
			want: []string{eslintLintFailPath, "-f", "json"},
		},
		{
			name: "keeps-explicit-format",
			args: []string{"--format", "stylish", eslintLintFailPath},
			want: []string{"--format", "stylish", eslintLintFailPath},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if !slices.Equal(prep.NormalizedArgs, tc.want) {
				t.Fatalf("unexpected args: want=%v got=%v", tc.want, prep.NormalizedArgs)
			}
		})
	}
}

func TestNpxEslintFilterStderrImmediate(t *testing.T) {
	f := NewNpxEslintFilter()
	d := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "network error\n"}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionImmediate {
		t.Fatalf("expected immediate stderr, got %#v", d)
	}
}

func TestNpxEslintFilterCollectsStdoutLineAndTick(t *testing.T) {
	f := NewNpxEslintFilter()
	mem := engine.NewOrderedSetBuffer()
	for _, ev := range []engine.Event{
		{Type: engine.EventLine, Stream: engine.StdoutStream, Line: "payload\n"},
		{Type: engine.EventTick, Stream: engine.StdoutStream},
	} {
		d := f.Process(ev, mem)
		if d.Action != engine.ActionCollect {
			t.Fatalf("expected collect for event=%v, got %#v", ev.Type, d)
		}
	}
}

func TestNpxEslintFilterStdoutEOFIgnores(t *testing.T) {
	f := NewNpxEslintFilter()
	d := f.Process(engine.Event{Type: engine.EventEOF, Stream: engine.StdoutStream}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionIgnore || d.Output != "" {
		t.Fatalf("expected ignore/no output on stdout EOF, got %#v", d)
	}
}

func TestSummarizeESLintOutput(t *testing.T) {
	raw := `[
  {
    "filePath": "/repo/src/lint_fail.js",
    "messages": [
      {"ruleId":"no-unused-vars","severity":2,"message":"'unused' is assigned a value but never used.","line":1,"column":7},
      {"ruleId":"semi","severity":2,"message":"Missing semicolon.","line":1,"column":17},
      {"ruleId":"semi","severity":1,"message":"Missing semicolon.","line":4,"column":15}
    ],
    "errorCount": 2,
    "warningCount": 1
  }
]`
	out, ok := summarizeESLintOutput(raw)
	if !ok {
		t.Fatalf("expected summary")
	}
	if !strings.Contains(out, "eslint: 2 errors, 1 warnings in 1 files") {
		t.Fatalf("unexpected header: %q", out)
	}
	if !strings.Contains(out, "top rules:\n- semi (2)\n- no-unused-vars (1)") {
		t.Fatalf("missing rule summary: %q", out)
	}
	if !strings.Contains(out, "top files:\n- "+eslintLintFailPath+" (3 issues)") {
		t.Fatalf("missing file summary: %q", out)
	}
}

func TestSummarizeESLintOutputNoIssues(t *testing.T) {
	raw := `[{"filePath":"/repo/src/lint_ok.js","messages":[],"errorCount":0,"warningCount":0}]`
	out, ok := summarizeESLintOutput(raw)
	if !ok || out != "" {
		t.Fatalf("unexpected no issue summary: ok=%v out=%q", ok, out)
	}
}

func TestNpxEslintProcessNoOutputOnSuccess(t *testing.T) {
	f := NewNpxEslintFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := `[{"filePath":"/repo/src/lint_ok.js","messages":[],"errorCount":0,"warningCount":0}]` + "\n"
	_ = mem.Add(raw, raw, 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream}, mem)
	if d.Action != engine.ActionIgnore || d.Output != "" {
		t.Fatalf("expected ignore/no output on success, got %#v", d)
	}
}

func TestNpxEslintProcessParseFallbackFlushesStrippedRaw(t *testing.T) {
	f := NewNpxEslintFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := strings.Join([]string{
		"Need to install the following packages:",
		"  eslint@9.0.0",
		"Ok to proceed? (y)",
		"not-json payload",
	}, "\n") + "\n"
	_ = mem.Add(raw, raw, 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush on parse fallback, got %#v", d)
	}
	if d.Output != "not-json payload\n" {
		t.Fatalf("expected stripped raw fallback output, got %q", d.Output)
	}
}
