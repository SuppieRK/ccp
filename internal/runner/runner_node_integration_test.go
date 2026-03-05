package runner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

const (
	errUnixNodeFixtureMsg = "shell script fixture is unix-specific"
	nodeShebangLine       = "#!/usr/bin/env sh"
	errWriteNodeScriptFmt = "write script: %v"
	errRegisterNodeFmt    = "register node filter: %v"
)

func TestRunnerNodeCompactionAndExitParity(t *testing.T) {
	skipWindowsNodeFixture(t)
	r, script := newNodeFixtureRunner(t, Options{}, nodeWarningScriptLines("echo 'payload line'"))

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "app.js"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.Count(out, "ExperimentalWarning") != 1 {
		t.Fatalf("expected folded warning retain-first, got %q", out)
	}
	if !strings.Contains(out, "[+1 similar warnings]") {
		t.Fatalf("expected warning fold count, got %q", out)
	}
	if !strings.Contains(out, "payload line") {
		t.Fatalf("expected payload retained, got %q", out)
	}
}

func TestRunnerNodeInteractivePassthroughExitParity(t *testing.T) {
	skipWindowsNodeFixture(t)
	r, script := newNodeFixtureRunner(t, Options{}, []string{
		nodeShebangLine,
		"echo 'interactive passthrough output'",
		"exit 6",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "-i"})
	})
	if code != 6 {
		t.Fatalf("expected exit code 6, got %d", code)
	}
	if !strings.Contains(out, "interactive passthrough output") {
		t.Fatalf("expected passthrough output, got %q", out)
	}
}

func TestRunnerNodeRawModeBypassesCompaction(t *testing.T) {
	skipWindowsNodeFixture(t)
	r, script := newNodeFixtureRunner(t, Options{Raw: true}, nodeWarningScriptLines())

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "app.js"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.Contains(out, "[+1 similar warnings]") {
		t.Fatalf("expected no compaction marker in raw mode, got %q", out)
	}
	if strings.Count(out, "ExperimentalWarning") != 2 {
		t.Fatalf("expected raw duplicate output, got %q", out)
	}
}

func skipWindowsNodeFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip(errUnixNodeFixtureMsg)
	}
}

func nodeWarningScriptLines(extra ...string) []string {
	lines := []string{
		nodeShebangLine,
		"echo '(node:111) ExperimentalWarning: The Fetch API is an experimental feature.'",
		"echo '(node:222) ExperimentalWarning: The Fetch API is an experimental feature.'",
	}
	lines = append(lines, extra...)
	lines = append(lines, "exit 0")
	return lines
}

func newNodeFixtureRunner(t *testing.T, opts Options, scriptLines []string) (*Runner, string) {
	t.Helper()
	work := t.TempDir()
	script := filepath.Join(work, "node")
	content := strings.Join(scriptLines, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf(errWriteNodeScriptFmt, err)
	}

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewNodeFilter()); err != nil {
		t.Fatalf(errRegisterNodeFmt, err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(opts, eng, reg), script
}
