package filters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestGolangciLintPrepareCases(t *testing.T) {
	f := NewGolangciLintFilter()
	cases := []struct {
		name             string
		args             []string
		want             []string
		forcePassthrough bool
		dispatchKey      string
	}{
		{
			name:        "injects-run-json",
			args:        []string{"./..."},
			want:        []string{"run", "--out-format", "json", "./..."},
			dispatchKey: golangciLintDispatchKey,
		},
		{
			name:        "keeps-run-and-injects-json",
			args:        []string{"run", "./..."},
			want:        []string{"run", "--out-format", "json", "./..."},
			dispatchKey: golangciLintDispatchKey,
		},
		{
			name:             "explicit-out-format-passthrough",
			args:             []string{"run", "--out-format", "json", "./..."},
			want:             []string{"run", "--out-format", "json", "./..."},
			forcePassthrough: true,
		},
		{
			name:             "unsupported-subcommand-passthrough",
			args:             []string{"version"},
			want:             []string{"version"},
			forcePassthrough: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if !slices.Equal(prep.NormalizedArgs, tc.want) {
				t.Fatalf("unexpected args: want=%v got=%v", tc.want, prep.NormalizedArgs)
			}
			if prep.ForcePassthrough != tc.forcePassthrough {
				t.Fatalf("unexpected passthrough: got=%v want=%v", prep.ForcePassthrough, tc.forcePassthrough)
			}
			if prep.DispatchKey != tc.dispatchKey {
				t.Fatalf("unexpected dispatch key: got=%q want=%q", prep.DispatchKey, tc.dispatchKey)
			}
		})
	}
}

func TestGolangciLintProcessStderrImmediate(t *testing.T) {
	f := NewGolangciLintFilter()
	d := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "config error\n"}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionImmediate || d.Output != "config error\n" {
		t.Fatalf("expected immediate stderr, got %#v", d)
	}
}

func TestSummarizeGolangciLintJSON(t *testing.T) {
	raw := `{"Issues":[
{"FromLinter":"errcheck","Text":"ignored error","Pos":{"Filename":"/repo/internal/api/server.go","Line":14,"Column":2}},
{"FromLinter":"revive","Text":"exported function should have comment","Pos":{"Filename":"/repo/internal/api/server.go","Line":20,"Column":1}},
{"FromLinter":"gosec","Text":"weak random source","Pos":{"Filename":"/repo/cmd/app/main.go","Line":9,"Column":5}}
]}`
	out, ok := summarizeGolangciLintJSON(raw)
	if !ok {
		t.Fatalf("expected summary")
	}
	for _, want := range []string{
		"golangci-lint: 3 issues in 2 files",
		"top linters:\n- errcheck (1)\n- gosec (1)\n- revive (1)",
		"- internal/api/server.go (2 issues)",
		"14:2 errcheck ignored error",
		"- cmd/app/main.go (1 issues)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestSummarizeGolangciLintJSONNoIssues(t *testing.T) {
	out, ok := summarizeGolangciLintJSON(`{"Issues":[]}`)
	if !ok || out != "" {
		t.Fatalf("unexpected no-issue result: ok=%v out=%q", ok, out)
	}
}

func TestGolangciLintProcessFallbackFlushesRaw(t *testing.T) {
	f := NewGolangciLintFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := "non-json lint output\n"
	_ = mem.Add(raw, raw, 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream}, mem)
	if d.Action != engine.ActionFlush || d.Output != raw {
		t.Fatalf("expected raw fallback flush, got %#v", d)
	}
}
