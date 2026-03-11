package filters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestRuffPrepareCases(t *testing.T) {
	f := NewRuffFilter()
	cases := []struct {
		name             string
		args             []string
		forcePassthrough bool
		dispatchKey      string
		wantArgs         []string
		ambiguous        bool
	}{
		{name: "default-dispatch", args: []string{"src"}, dispatchKey: ruffDispatchKey, wantArgs: []string{"check", "--output-format", "json", "src"}},
		{name: "empty-dispatch", args: nil, dispatchKey: ruffDispatchKey, wantArgs: []string{"check", "--output-format", "json", "."}},
		{name: "check-dispatch", args: []string{"check", "src"}, dispatchKey: ruffDispatchKey, wantArgs: []string{"check", "--output-format", "json", "src"}},
		{name: "structured-passthrough", args: []string{"check", "--output-format", "json", "src"}, forcePassthrough: true, ambiguous: true},
		{name: "format-passthrough", args: []string{"format", "src"}, forcePassthrough: true},
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
			if tc.wantArgs != nil && !slices.Equal(prep.NormalizedArgs, tc.wantArgs) {
				t.Fatalf("unexpected args: got=%#v want=%#v", prep.NormalizedArgs, tc.wantArgs)
			}
			if prep.Ambiguous != tc.ambiguous {
				t.Fatalf("unexpected ambiguous: got=%v want=%v", prep.Ambiguous, tc.ambiguous)
			}
		})
	}
}

func TestRuffProcessStderrImmediate(t *testing.T) {
	f := NewRuffFilter()
	d := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "config error\n"}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionImmediate || d.Output != "config error\n" {
		t.Fatalf("expected immediate stderr, got %#v", d)
	}
}

func TestSummarizeRuffJSON(t *testing.T) {
	raw := strings.Join([]string{
		"[",
		`  {"code":"F401","message":"` + "`os` imported but unused" + `","location":{"row":1,"column":8},"filename":"src/main.py","fix":{"applicability":"safe"}},`,
		`  {"code":"F401","message":"` + "`sys` imported but unused" + `","location":{"row":2,"column":8},"filename":"src/main.py","fix":null},`,
		`  {"code":"E501","message":"Line too long (100 > 88 characters)","location":{"row":10,"column":89},"filename":"src/utils.py","fix":null}`,
		"]",
	}, "\n")
	out, ok := summarizeRuffJSON(raw)
	if !ok {
		t.Fatal("expected summary")
	}
	for _, want := range []string{
		"ruff: 3 issues in 2 files (1 fixable)",
		"- F401 (2)",
		"- src/main.py (2 issues)",
		"1:8 F401",
		"10:89 E501",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestSummarizeRuffJSONFallback(t *testing.T) {
	if _, ok := summarizeRuffJSON("not json\n"); ok {
		t.Fatal("expected fallback")
	}
}
