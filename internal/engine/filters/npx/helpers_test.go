package npxfilters

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

func TestNpxSubfilterMethodCoverage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		filter      engine.ToolFilter
		wantTool    string
		wantHorizon int
	}{
		{name: "eslint", filter: NewNpxEslintFilter(), wantTool: "npx eslint", wantHorizon: 0},
		{name: "node", filter: NewNpxNodeFilter(), wantTool: "npx node", wantHorizon: 4096},
		{name: "prettier", filter: NewNpxPrettierFilter(), wantTool: "npx prettier", wantHorizon: 0},
		{name: "prisma", filter: NewNpxPrismaFilter(), wantTool: "npx prisma", wantHorizon: 0},
		{name: "tsc", filter: NewNpxTscFilter(), wantTool: "npx tsc", wantHorizon: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertNpxSubfilterBasics(t, tc.filter, tc.wantTool, tc.wantHorizon)
		})
	}
}

func TestNpxNodeHelperDelegates(t *testing.T) {
	t.Parallel()

	if !filtercommon.NodeIsInteractiveInvocation([]string{"-i"}) {
		t.Fatal("expected interactive invocation detection")
	}
	if !filtercommon.NodeIsUnhandledFailure("Unhandled rejection: TypeError: boom") {
		t.Fatal("expected unhandled failure detection")
	}
	if filtercommon.NodeCanonicalLine("(node:123) Warning: x") == "" {
		t.Fatal("expected canonical line")
	}
	if !filtercommon.NodeLowConfidenceOutput(string([]byte{'a', 0, 'b'})) {
		t.Fatal("expected low confidence output for binary-like payload")
	}
}

func assertNpxSubfilterBasics(t *testing.T, filter engine.ToolFilter, wantTool string, wantHorizon int) {
	t.Helper()
	if got := filter.Tool(); got != wantTool {
		t.Fatalf("Tool() = %q, want %q", got, wantTool)
	}
	if got := filter.Aliases(); got != nil {
		t.Fatalf("Aliases() = %v, want nil", got)
	}
	prep := filter.Prepare([]string{"--x"})
	if len(prep.NormalizedArgs) == 0 {
		t.Fatalf("Prepare() produced empty args: %#v", prep)
	}
	if filter.ContextKey(engine.Event{CommandID: "c", Tool: wantTool, Stream: engine.StdoutStream}) == "" {
		t.Fatal("ContextKey() returned empty key")
	}
	if got := filter.MaskingHorizon(); got != wantHorizon {
		t.Fatalf("MaskingHorizon() = %d, want %d", got, wantHorizon)
	}
}

func TestNpxNodeProcessFiltersInstallPrompt(t *testing.T) {
	t.Parallel()

	f := NewNpxNodeFilter()
	d := f.Process(engine.Event{
		Type:   engine.EventLine,
		Stream: engine.StdoutStream,
		Line:   "Need to install the following packages:\n",
	}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionIgnore {
		t.Fatalf("expected install prompt to be ignored, got %#v", d)
	}
}

func TestNpxPrismaProcessEOFAndStderrCoverage(t *testing.T) {
	t.Parallel()

	f := NewNpxPrismaFilter()
	mem := engine.NewOrderedSetBuffer()
	if d := f.Process(engine.Event{Type: engine.EventEOF, Stream: engine.StdoutStream}, mem); d.Action != engine.ActionIgnore {
		t.Fatalf("expected EOF ignore, got %#v", d)
	}
	if d := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "prisma warning\n"}, mem); d.Action != engine.ActionImmediate {
		t.Fatalf("expected stderr immediate, got %#v", d)
	}
}
