package filters

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const (
	findFlushErrFmt = "expected flush, got %q"
	findHiddenName  = ".hidden"
	findKeepGo      = "keep.go"
	findSkipTmp     = "skip.tmp"
)

func TestFindFilterMetadataAndPrepare(t *testing.T) {
	f := NewFindFilter()
	if f.Tool() != "find" {
		t.Fatalf("expected tool find, got %q", f.Tool())
	}
	if got := f.Aliases(); len(got) != 0 {
		t.Fatalf("expected no aliases, got %#v", got)
	}

	prep := f.Prepare([]string{".", "-name", "*.go", "-type", "f", "--max-results", "7", "--heartbeat"})
	if prep.DispatchKey == "" {
		t.Fatal("expected dispatch key")
	}
	for _, want := range []string{"pattern=*.go", "type=f", "max=7", "hidden=1", "heartbeat=1"} {
		if !strings.Contains(prep.DispatchKey, want) {
			t.Fatalf("expected %q in dispatch key, got %q", want, prep.DispatchKey)
		}
	}
	if prep.PreferredSubstitution != "fd" {
		t.Fatalf("expected preferred substitution fd, got %q", prep.PreferredSubstitution)
	}
	if len(prep.FallbackArgs) == 0 {
		t.Fatal("expected fallback args")
	}
}

func TestFindPrepareDefaultRootAndExpressionAliases(t *testing.T) {
	f := NewFindFilter()
	prep := f.Prepare([]string{"-iname", "*.go", "-type", "d", "--max_results", "9", "--all"})

	if len(prep.NormalizedArgs) == 0 || prep.NormalizedArgs[0] != "." {
		t.Fatalf("expected default root '.' in normalized args, got %#v", prep.NormalizedArgs)
	}
	for _, want := range []string{"pattern=*.go", "type=d", "max=9", "root=.", "hidden=1"} {
		if !strings.Contains(prep.DispatchKey, want) {
			t.Fatalf("expected %q in dispatch key, got %q", want, prep.DispatchKey)
		}
	}
}

func TestFindPrepareUnsafeExpressionsDisableFDSubstitution(t *testing.T) {
	f := NewFindFilter()
	prep := f.Prepare([]string{".", "-name", "*.go", "-exec", "echo", "{}", ";"})
	if prep.PreferredSubstitution != "" {
		t.Fatalf("expected no preferred substitution for unsafe expression, got %q", prep.PreferredSubstitution)
	}
	if len(prep.PreferredArgs) != 0 {
		t.Fatalf("expected no preferred args for unsafe expression, got %#v", prep.PreferredArgs)
	}
	if len(prep.FallbackArgs) != 0 {
		t.Fatalf("expected no fallback args when substitution is disabled, got %#v", prep.FallbackArgs)
	}
}

func TestFindFilterZeroMatchOutput(t *testing.T) {
	f := NewFindFilter()
	mem := engine.NewOrderedSetBuffer()

	d := f.Process(engine.Event{Type: engine.EventExit, Tool: "find", Stream: engine.StdoutStream, Dispatch: "find|pattern=*.go|max=5|type=f|root=.", ExitCode: 0}, mem)
	if d.Action != engine.ActionIgnore {
		t.Fatalf("expected ignore, got %q", d.Action)
	}
	if d.Output != "" {
		t.Fatalf("expected empty output, got %q", d.Output)
	}
}

func TestFindFilterDirectoryModeRendering(t *testing.T) {
	f := NewFindFilter()
	mem := engine.NewOrderedSetBuffer()
	lines := []string{
		"./pkg/c\n",
		"./pkg/a\n",
		"./pkg/b\n",
	}
	for i, line := range lines {
		_ = mem.Add(line, line, uint64(i+1))
	}

	d := f.Process(engine.Event{Type: engine.EventExit, Tool: "find", Stream: engine.StdoutStream, Dispatch: "find|pattern=*|type=d|max=2|root=."}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf(findFlushErrFmt, d.Action)
	}
	if !strings.Contains(d.Output, "pkg/a/") || !strings.Contains(d.Output, "pkg/b/") {
		t.Fatalf("expected sorted directory rendering, got:\n%s", d.Output)
	}
	if !strings.Contains(d.Output, "+1 more") {
		t.Fatalf("expected bounded directory marker, got:\n%s", d.Output)
	}
}

func TestFindFilterCompactionDeterministicAndBounded(t *testing.T) {
	f := NewFindFilter()
	mem := engine.NewOrderedSetBuffer()
	input := []string{
		"./pkg/b/b.go\n",
		"./pkg/a/a.go\n",
		"./pkg/a/z.txt\n",
		"./pkg/a/a.go\n",
	}
	for i, line := range input {
		_ = mem.Add(line, line, uint64(i+1))
	}

	d := f.Process(engine.Event{Type: engine.EventExit, Tool: "find", Stream: engine.StdoutStream, Dispatch: "find|pattern=*|type=f|max=2|root=.", ExitCode: 0}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf(findFlushErrFmt, d.Action)
	}
	if !strings.Contains(d.Output, "pkg/a/") {
		t.Fatalf("expected grouped directory output, got:\n%s", d.Output)
	}
	if !strings.Contains(d.Output, "+1 more") {
		t.Fatalf("expected remainder marker, got:\n%s", d.Output)
	}
	if strings.Contains(d.Output, "summary:") {
		t.Fatalf("did not expect summary line, got:\n%s", d.Output)
	}
}

func TestFindFilterCanSuppressHiddenWhenExplicitlyDisabled(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "vendor", "x"))
	mustWriteFile(t, filepath.Join(root, "vendor", "x", "a.go"), "package x")
	mustMkdirAll(t, filepath.Join(root, findHiddenName))
	mustWriteFile(t, filepath.Join(root, findHiddenName, "a.go"), "package x")
	mustWriteFile(t, filepath.Join(root, findKeepGo), "package main")
	mustWriteFile(t, filepath.Join(root, findSkipTmp), "x")

	f := NewFindFilter()
	mem := engine.NewOrderedSetBuffer()
	lines := []string{
		filepath.Join(root, "vendor", "x", "a.go") + "\n",
		filepath.Join(root, findHiddenName, "a.go") + "\n",
		filepath.Join(root, findKeepGo) + "\n",
		filepath.Join(root, findSkipTmp) + "\n",
	}
	for i, line := range lines {
		_ = mem.Add(line, line, uint64(i+1))
	}
	dispatch := "find|pattern=*|type=f|max=50|root=" + root + "|hidden=0"
	d := f.Process(engine.Event{Type: engine.EventExit, Tool: "find", Stream: engine.StdoutStream, Dispatch: dispatch, ExitCode: 0}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf(findFlushErrFmt, d.Action)
	}
	if !strings.Contains(d.Output, findKeepGo) {
		t.Fatalf("expected keep.go in output, got:\n%s", d.Output)
	}
	if !strings.Contains(d.Output, "vendor") || !strings.Contains(d.Output, findSkipTmp) {
		t.Fatalf("expected non-hidden paths to remain, got:\n%s", d.Output)
	}
	if strings.Contains(d.Output, findHiddenName) {
		t.Fatalf("expected hidden paths to be suppressed, got:\n%s", d.Output)
	}
}

func TestFindFilterLowConfidencePassthrough(t *testing.T) {
	f := NewFindFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := "abc\x00def\n"
	_ = mem.Add(raw, raw, 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Tool: "find", Stream: engine.StdoutStream, Dispatch: "find|pattern=*|type=f|max=5|root=.", ExitCode: 0}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf(findFlushErrFmt, d.Action)
	}
	if d.Output != raw {
		t.Fatalf("expected passthrough raw output, got %q", d.Output)
	}
}

func TestFindFilterOptionalHeartbeat(t *testing.T) {
	f := NewFindFilter()
	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add("./a\n", "./a\n", 1)
	d := f.Process(engine.Event{Type: engine.EventTick, Tool: "find", Stream: engine.StdoutStream, Dispatch: "find|pattern=*|type=f|max=5|root=.|heartbeat=1"}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf(findFlushErrFmt, d.Action)
	}
	if !strings.Contains(d.Output, "scanned 1 paths") {
		t.Fatalf("unexpected heartbeat output: %q", d.Output)
	}
}

func TestFindFilterCompressionThreshold(t *testing.T) {
	f := NewFindFilter()
	mem := engine.NewOrderedSetBuffer()
	for i := 0; i < 20; i++ {
		line := "./src/pkg/file" + strconv.Itoa(i) + ".go\n"
		_ = mem.Add(line, line, uint64(i+1))
	}
	d := f.Process(engine.Event{Type: engine.EventExit, Tool: "find", Stream: engine.StdoutStream, Dispatch: "find|pattern=*|type=f|max=5|root=.", ExitCode: 0}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf(findFlushErrFmt, d.Action)
	}
	rawLines := 20
	kept := strings.Count(strings.TrimSpace(d.Output), "\n") + 1
	if kept >= rawLines {
		t.Fatalf("expected compaction to reduce lines, raw=%d kept=%d\noutput:\n%s", rawLines, kept, d.Output)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
