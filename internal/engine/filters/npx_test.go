package filters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const npxTscDispatch = "npx tsc"

func TestNPXParentMetadataAndAliases(t *testing.T) {
	f := NewNPXFilter()
	if f.Tool() != "npx" {
		t.Fatalf("expected npx tool, got %q", f.Tool())
	}
	want := []string{"npx.cmd", "./npx.cmd", "npx.exe", "./npx.exe"}
	if !slices.Equal(f.Aliases(), want) {
		t.Fatalf("unexpected aliases: want=%#v got=%#v", want, f.Aliases())
	}
}

func TestNPXPrepareDispatchesAllowlistRoutes(t *testing.T) {
	f := NewNPXFilter()
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"tsc", "--noEmit"}, want: npxTscDispatch},
		{args: []string{"typescript", "--noEmit"}, want: npxTscDispatch},
		{args: []string{"eslint", "."}, want: "npx eslint"},
		{args: []string{"prettier", "--check", "."}, want: "npx prettier"},
		{args: []string{"prisma", "generate"}, want: "npx prisma"},
		{args: []string{"node", "app.js"}, want: "npx node"},
	}
	for _, tc := range tests {
		prep := f.Prepare(tc.args)
		if prep.ForcePassthrough {
			t.Fatalf("expected route, got passthrough for args=%#v", tc.args)
		}
		if prep.DispatchKey != tc.want {
			t.Fatalf("args=%#v expected dispatch=%q got=%q", tc.args, tc.want, prep.DispatchKey)
		}
	}
}

func TestNPXPrepareUnsupportedOrMalformedPassthrough(t *testing.T) {
	f := NewNPXFilter()
	tests := [][]string{
		{},
		{""},
		{"unknown-tool", "x"},
		{"-y"},
	}
	for _, args := range tests {
		prep := f.Prepare(args)
		if !prep.ForcePassthrough {
			t.Fatalf("expected passthrough for args=%#v", args)
		}
	}
}

func TestNPXPreparePackageFlagsForcePassthrough(t *testing.T) {
	f := NewNPXFilter()
	tests := [][]string{
		{"-p", "cowsay", "lolcat"},
		{"--package", "cowsay", "lolcat"},
		{"--package=cowsay", "lolcat"},
		{"-p=cowsay", "lolcat"},
	}
	for _, args := range tests {
		prep := f.Prepare(args)
		if !prep.ForcePassthrough {
			t.Fatalf("expected low-confidence passthrough for args=%#v", args)
		}
	}
}

func TestNPXPrepareLeadingFlagsAndDelimiterResolveTool(t *testing.T) {
	f := NewNPXFilter()
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"-y", "eslint", "."}, want: "npx eslint"},
		{args: []string{"--", "prettier", "--check", "."}, want: "npx prettier"},
		{args: []string{"-q", "--", "node", "app.js"}, want: "npx node"},
	}
	for _, tc := range tests {
		prep := f.Prepare(tc.args)
		if prep.ForcePassthrough {
			t.Fatalf("expected routed dispatch for args=%#v, got passthrough", tc.args)
		}
		if prep.DispatchKey != tc.want {
			t.Fatalf("args=%#v expected dispatch=%q got=%q", tc.args, tc.want, prep.DispatchKey)
		}
	}
}

func TestNPXRoutedFilterSuppressesWrapperNoise(t *testing.T) {
	f := NewNPXFilter()
	mem := engine.NewOrderedSetBuffer()
	lines := []string{
		"Need to install the following packages:\n",
		"  eslint@9.0.0\n",
		"Ok to proceed? (y)\n",
		"npm WARN exec The following package was not found and will be installed\n",
		"/repo/src/app.ts:1:1  error  Unexpected any\n",
	}
	for i, line := range lines {
		_ = mem.Add(line, line, uint64(i+1))
	}
	d := f.Process(engine.Event{Type: engine.EventExit, Tool: "npx", Dispatch: "npx eslint", Stream: engine.StdoutStream, ExitCode: 1}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush, got %q", d.Action)
	}
	for _, noise := range []string{"Need to install", "Ok to proceed", "npm WARN exec"} {
		if strings.Contains(d.Output, noise) {
			t.Fatalf("expected wrapper noise suppressed, got %q", d.Output)
		}
	}
	if !strings.Contains(d.Output, "Unexpected any") {
		t.Fatalf("expected payload retained, got %q", d.Output)
	}
}

func TestNPXRoutedFilterPreservesStderr(t *testing.T) {
	f := NewNPXFilter()
	d := f.Process(engine.Event{Type: engine.EventLine, Tool: "npx", Dispatch: npxTscDispatch, Stream: engine.StderrStream, Line: "network error\n"}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionImmediate {
		t.Fatalf("expected immediate stderr output, got %q", d.Action)
	}
	if d.Output != "network error\n" {
		t.Fatalf("unexpected stderr output: %q", d.Output)
	}
}

func TestNPXNodeDelegatesToNodeCompaction(t *testing.T) {
	f := NewNPXFilter()
	mem := engine.NewOrderedSetBuffer()
	lines := []string{
		"(node:111) ExperimentalWarning: x\n",
		"(node:222) ExperimentalWarning: x\n",
		"payload\n",
	}
	for i, line := range lines {
		_ = mem.Add(line, line, uint64(i+1))
	}
	d := f.Process(engine.Event{Type: engine.EventExit, Tool: "npx", Dispatch: "npx node", Stream: engine.StdoutStream}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush, got %q", d.Action)
	}
	if strings.Count(d.Output, "ExperimentalWarning") != 1 {
		t.Fatalf("expected folded warning retain-first, got %q", d.Output)
	}
	if !strings.Contains(d.Output, "[+1 similar warnings]") {
		t.Fatalf("expected folded warning count, got %q", d.Output)
	}
	if !strings.Contains(d.Output, "payload") {
		t.Fatalf("expected payload retained, got %q", d.Output)
	}
}

func TestNPXRuntimeFallbackForUnknownDispatchShapes(t *testing.T) {
	f := NewNPXFilter()
	tests := []string{"", "eslint", "npx unknown"}
	for _, dispatch := range tests {
		t.Run("dispatch="+dispatch, func(t *testing.T) {
			ev := engine.Event{
				Type:     engine.EventLine,
				Tool:     "npx",
				Dispatch: dispatch,
				Stream:   engine.StdoutStream,
				Line:     "payload\n",
			}
			d := f.Process(ev, engine.NewOrderedSetBuffer())
			if d.Action != engine.ActionImmediate {
				t.Fatalf("expected noop immediate fallback for dispatch=%q, got %#v", dispatch, d)
			}
			if d.Output != "payload\n" {
				t.Fatalf("expected unchanged line output for dispatch=%q, got %q", dispatch, d.Output)
			}
		})
	}
}
