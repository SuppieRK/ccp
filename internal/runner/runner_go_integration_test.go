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
	errUnixGoFixtureMsg = "shell script fixture is unix-specific"
	goShebangLine       = "#!/usr/bin/env sh"
	goUnexpectedInvoke  = "echo 'unexpected invocation' 1>&2"
	goExitThreeLine     = "exit 3"
	errWriteGoScriptFmt = "write script: %v"
	errRegisterGoFmt    = "register go filter: %v"
)

func TestRunnerGoTestCompaction(t *testing.T) {
	skipWindowsGoFixture(t)
	r, script := newGoFixtureRunner(t, Options{}, goTestScriptLines(true))

	out, code := captureCombined(t, func() int { return r.Run([]string{script, "test", "./..."}) })
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, "go test: 2 passed, 1 no-test-files") {
		t.Fatalf("expected go test summary, got %q", out)
	}
}

func TestRunnerGoStderrVisibilityAndExitParity(t *testing.T) {
	skipWindowsGoFixture(t)
	r, script := newGoFixtureRunner(t, Options{}, []string{
		goShebangLine,
		"if [ \"$1\" = \"build\" ]; then",
		"  echo 'compile failed' 1>&2",
		"  exit 9",
		"fi",
		goUnexpectedInvoke,
		goExitThreeLine,
	})

	out, code := captureCombined(t, func() int { return r.Run([]string{script, "build", "./..."}) })
	if code != 9 {
		t.Fatalf("expected exit 9, got %d", code)
	}
	if !strings.Contains(out, "compile failed") {
		t.Fatalf("expected stderr retained, got %q", out)
	}
}

func TestRunnerGoRawBypass(t *testing.T) {
	skipWindowsGoFixture(t)
	r, script := newGoFixtureRunner(t, Options{Raw: true}, goTestScriptLines(false))

	out, code := captureCombined(t, func() int { return r.Run([]string{script, "test", "./..."}) })
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if strings.Contains(out, "go test: ") {
		t.Fatalf("expected raw output in raw mode, got %q", out)
	}
	if !strings.Contains(out, "ok   github.com/acme/p1 0.101s") {
		t.Fatalf("expected native output, got %q", out)
	}
}

func skipWindowsGoFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip(errUnixGoFixtureMsg)
	}
}

func goTestScriptLines(includeNoTests bool) []string {
	lines := []string{
		goShebangLine,
		"if [ \"$1\" = \"test\" ]; then",
		"  echo 'ok   github.com/acme/p1 0.101s'",
		"  echo 'ok   github.com/acme/p2 0.201s'",
	}
	if includeNoTests {
		lines = append(lines, "  echo '?    github.com/acme/p3 [no test files]'")
	}
	lines = append(lines,
		"  exit 0",
		"fi",
		goUnexpectedInvoke,
		goExitThreeLine,
	)
	return lines
}

func newGoFixtureRunner(t *testing.T, opts Options, scriptLines []string) (*Runner, string) {
	t.Helper()
	work := t.TempDir()
	script := filepath.Join(work, "go")
	content := strings.Join(scriptLines, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf(errWriteGoScriptFmt, err)
	}

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewGoToolFilter()); err != nil {
		t.Fatalf(errRegisterGoFmt, err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(opts, eng, reg), script
}
