package filters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

const (
	nodeDispatchRuntime           = "node|mode=runtime"
	nodeExperimentalWarningSample = "(node:123) ExperimentalWarning: x"
	nodeCompactionExpected        = "expected compaction"
	nodeUnhandledRejectionWarning = "UnhandledPromiseRejectionWarning: boom\n"
)

func TestNodeFilterMetadataAndPrepare(t *testing.T) {
	f := NewNodeFilter()
	if f.Tool() != "node" {
		t.Fatalf("expected node tool, got %q", f.Tool())
	}
	wantAliases := []string{"node.exe", "./node.exe", "node.cmd", "./node.cmd"}
	if !slices.Equal(f.Aliases(), wantAliases) {
		t.Fatalf("unexpected aliases: want=%#v got=%#v", wantAliases, f.Aliases())
	}
	prep := f.Prepare([]string{"app.js"})
	if prep.ForcePassthrough {
		t.Fatal("expected script invocation to be filtered")
	}
	if prep.DispatchKey != nodeDispatchRuntime {
		t.Fatalf("unexpected dispatch key: %q", prep.DispatchKey)
	}
	if !slices.Equal(prep.NormalizedArgs, []string{"app.js"}) {
		t.Fatalf("unexpected args: %#v", prep.NormalizedArgs)
	}
}

func TestNodeFilterPrepareInteractivePassthrough(t *testing.T) {
	f := NewNodeFilter()
	tests := [][]string{
		nil,
		{},
		{"-i"},
		{"--interactive"},
		{"--trace-warnings"},
	}
	for _, tc := range tests {
		prep := f.Prepare(tc)
		if !prep.ForcePassthrough {
			t.Fatalf("expected passthrough for args %#v", tc)
		}
	}

	if prep := f.Prepare([]string{"-e", "console.log('ok')"}); prep.ForcePassthrough {
		t.Fatalf("expected eval mode to stay filtered")
	}
}

func TestNodeFilterSharedContextAcrossStreams(t *testing.T) {
	f := NewNodeFilter()
	stdout := f.ContextKey(engine.Event{CommandID: "node app.js", Tool: "node", Stream: engine.StdoutStream})
	stderr := f.ContextKey(engine.Event{CommandID: "node app.js", Tool: "node", Stream: engine.StderrStream})
	if stdout != stderr {
		t.Fatalf("expected shared context key, got %q != %q", stdout, stderr)
	}
}

func TestNormalizeNodeWarningKeyPIDVariantsFold(t *testing.T) {
	a := filtercommon.NodeNormalizeWarningKey(nodeExperimentalWarningSample)
	b := filtercommon.NodeNormalizeWarningKey("(node:456) ExperimentalWarning: x")
	if a != b {
		t.Fatalf("expected pid-variant warning keys to fold, got %q != %q", a, b)
	}
}

func TestNormalizeNodeWarningKeyDoesNotAffectNonRuntimePayload(t *testing.T) {
	in := "(123) user payload starts with parens"
	if out := filtercommon.NodeNormalizeWarningKey(in); out != strings.ToLower(in) {
		t.Fatalf("unexpected normalization for non-runtime payload: %q", out)
	}
}

func TestNodeCompactionSuppressesProgressAndFoldsWarnings(t *testing.T) {
	raw := strings.Join([]string{
		nodeExperimentalWarningSample,
		"(node:456) ExperimentalWarning: x",
		"\r⠋ loading...",
		"application payload",
	}, "\n") + "\n"
	out, ok := compactNodeOutput(raw)
	if !ok {
		t.Fatal(nodeCompactionExpected)
	}
	if strings.Contains(out, "⠋") {
		t.Fatalf("expected spinner/progress suppression, got %q", out)
	}
	if !strings.Contains(out, nodeExperimentalWarningSample) {
		t.Fatalf("expected retain-first warning, got %q", out)
	}
	if !strings.Contains(out, "[+1 similar warnings]") {
		t.Fatalf("expected folded warning count, got %q", out)
	}
	if !strings.Contains(out, "application payload") {
		t.Fatalf("expected payload retained, got %q", out)
	}
}

func TestNodeCompactionFoldsESMModeWarning(t *testing.T) {
	raw := strings.Join([]string{
		`Warning: To load an ES module, set "type": "module" in the package.json`,
		`Warning: To load an ES module, set "type": "module" in the package.json`,
	}, "\n") + "\n"
	out, ok := compactNodeOutput(raw)
	if !ok {
		t.Fatal(nodeCompactionExpected)
	}
	if !strings.Contains(out, "[+1 similar warnings]") {
		t.Fatalf("expected esm warning fold, got %q", out)
	}
}

func TestNodeCompactionRetainsFailureDiagnostics(t *testing.T) {
	raw := strings.Join([]string{
		"Error: boom",
		"    at Object.<anonymous> (/repo/app.js:1:1)",
		"Caused by: root-cause",
	}, "\n") + "\n"
	out, ok := compactNodeOutput(raw)
	if !ok {
		t.Fatal(nodeCompactionExpected)
	}
	if !strings.Contains(out, "Error: boom") || !strings.Contains(out, "at Object.<anonymous>") {
		t.Fatalf("expected failure lines retained, got %q", out)
	}
}

func TestNodeUnhandledRejectionFlushCases(t *testing.T) {
	f := NewNodeFilter()
	cases := []struct {
		name         string
		lines        []string
		wantContains []string
		wantExact    string
	}{
		{
			name:  "buffered-warnings-flushed",
			lines: []string{"(node:111) ExperimentalWarning: x\n", nodeUnhandledRejectionWarning},
			wantContains: []string{
				"ExperimentalWarning",
				"UnhandledPromiseRejection",
			},
		},
		{
			name:      "single-line-fast-path",
			lines:     []string{nodeUnhandledRejectionWarning},
			wantExact: nodeUnhandledRejectionWarning,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			for i, line := range tc.lines {
				_ = mem.Add(line, line, uint64(i+1))
			}
			d := f.Process(engine.Event{
				Type:     engine.EventLine,
				Tool:     "node",
				Dispatch: nodeDispatchRuntime,
				Line:     nodeUnhandledRejectionWarning,
				Stream:   engine.StderrStream,
			}, mem)
			if d.Action != engine.ActionFlush {
				t.Fatalf("expected flush on unhandled rejection, got %q", d.Action)
			}
			if tc.wantExact != "" && d.Output != tc.wantExact {
				t.Fatalf("expected exact output %q, got %q", tc.wantExact, d.Output)
			}
			for _, want := range tc.wantContains {
				if !strings.Contains(d.Output, want) {
					t.Fatalf("expected output to contain %q, got %q", want, d.Output)
				}
			}
		})
	}
}

func TestNodeFilterExitFallbackOnLowConfidence(t *testing.T) {
	f := NewNodeFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := "abc\x00def\n"
	_ = mem.Add(raw, raw, 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Tool: "node", Dispatch: nodeDispatchRuntime, ExitCode: 0}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush, got %q", d.Action)
	}
	if d.Output != raw {
		t.Fatalf("expected raw fallback, got %q", d.Output)
	}
}

func TestNodeFilterCollectsTickAndEOF(t *testing.T) {
	f := NewNodeFilter()
	mem := engine.NewOrderedSetBuffer()
	for _, ev := range []engine.Event{
		{Type: engine.EventTick, Tool: "node", Dispatch: nodeDispatchRuntime, Stream: engine.StdoutStream},
		{Type: engine.EventEOF, Tool: "node", Dispatch: nodeDispatchRuntime, Stream: engine.StdoutStream},
	} {
		d := f.Process(ev, mem)
		if d.Action != engine.ActionCollect {
			t.Fatalf("expected collect for event=%v, got %#v", ev.Type, d)
		}
	}
}

func TestNodeFilterCollectsNonFailureLine(t *testing.T) {
	f := NewNodeFilter()
	mem := engine.NewOrderedSetBuffer()
	d := f.Process(engine.Event{
		Type:     engine.EventLine,
		Tool:     "node",
		Dispatch: nodeDispatchRuntime,
		Stream:   engine.StdoutStream,
		Line:     "regular payload line\n",
	}, mem)
	if d.Action != engine.ActionCollect {
		t.Fatalf("expected collect for non-failure line, got %#v", d)
	}
}

func TestNodeFilterExitEmptyBufferIgnores(t *testing.T) {
	f := NewNodeFilter()
	d := f.Process(engine.Event{
		Type:     engine.EventExit,
		Tool:     "node",
		Dispatch: nodeDispatchRuntime,
		ExitCode: 0,
	}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionIgnore || d.Output != "" {
		t.Fatalf("expected ignore with empty output for empty exit buffer, got %#v", d)
	}
}

func TestNodeFilterNonZeroExitWithEmptyCompactionFallsBackToRaw(t *testing.T) {
	f := NewNodeFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := "\r⠋ loading...\n"
	_ = mem.Add(raw, "k", 1)
	d := f.Process(engine.Event{
		Type:     engine.EventExit,
		Tool:     "node",
		Dispatch: nodeDispatchRuntime,
		ExitCode: 1,
	}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected raw flush fallback for non-zero exit with empty compact output, got %#v", d)
	}
	if d.Output != raw {
		t.Fatalf("expected raw output fallback, got %q", d.Output)
	}
}

func TestNodeCompactionThresholdOnWarningNoise(t *testing.T) {
	lines := make([]string, 0, 40)
	for i := 0; i < 30; i++ {
		lines = append(lines, "(node:12345) ExperimentalWarning: The Fetch API is an experimental feature.")
	}
	lines = append(lines, "ok payload")
	raw := strings.Join(lines, "\n") + "\n"
	out, ok := compactNodeOutput(raw)
	if !ok {
		t.Fatal(nodeCompactionExpected)
	}
	rawLines := countNonEmptyLines(raw)
	outLines := countNonEmptyLines(out)
	drop := float64(rawLines-outLines) / float64(rawLines)
	if drop < 0.15 {
		t.Fatalf("expected drop >= 0.15, got %.2f (raw=%d out=%d)", drop, rawLines, outLines)
	}
}
