package gofilters

import (
	"slices"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestGoCommonHelperFunctions(t *testing.T) {
	t.Parallel()

	if !isFailureMarker("--- FAIL: TestX (0.00s)") {
		t.Fatal("expected fail marker detection")
	}
	cases := []struct {
		line string
		want bool
	}{
		{line: "--- FAIL: TestX (0.00s)", want: true},
		{line: "ok github.com/acme/p 0.01s", want: false},
	}
	for _, tc := range cases {
		if got := shouldEmitFailureContext(tc.line); got != tc.want {
			t.Fatalf("shouldEmitFailureContext(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

func TestJoinTailAndFilteredFailureContext(t *testing.T) {
	t.Parallel()

	lines := []string{"one\n", "two\n", "--- FAIL: TestY\n", "ctx\n"}
	if got := joinTail(lines, 2); got != "--- FAIL: TestY\nctx\n" {
		t.Fatalf("joinTail() mismatch: %q", got)
	}
	if got := filteredFailureContext(lines, 4); got != "one\ntwo\nctx\n" {
		t.Fatalf("filteredFailureContext() mismatch: %q", got)
	}
}

func TestMapKeysSorted(t *testing.T) {
	t.Parallel()

	keys := mapKeysSorted(map[string]struct{}{"b": {}, "a": {}, "c": {}})
	if !slices.Equal(keys, []string{"a", "b", "c"}) {
		t.Fatalf("mapKeysSorted() mismatch: %#v", keys)
	}
}

func TestGoSubfilterMethodCoverage(t *testing.T) {
	t.Parallel()

	build := NewBuildFilter()
	testf := NewTestFilter()
	cases := []struct {
		name   string
		filter engine.ToolFilter
		tool   string
	}{
		{name: "build", filter: build, tool: "go build"},
		{name: "test", filter: testf, tool: "go test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.filter.Tool(); got != tc.tool {
				t.Fatalf("Tool() = %q, want %q", got, tc.tool)
			}
			if got := tc.filter.Aliases(); got != nil {
				t.Fatalf("Aliases() = %v, want nil", got)
			}
			if tc.filter.ContextKey(engine.Event{CommandID: "c", Tool: tc.tool, Stream: engine.StdoutStream}) == "" {
				t.Fatal("ContextKey() returned empty key")
			}
			if got := tc.filter.MaskingHorizon(); got != 0 {
				t.Fatalf("MaskingHorizon() = %d, want 0", got)
			}
		})
	}
}
