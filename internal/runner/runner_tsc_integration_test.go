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

func TestRunnerTscCompactsSupportedOutputAndPreservesExitCode(t *testing.T) {
	skipWindowsTscFixture(t)
	r, script := newTscFixtureRunner(t, Options{}, []string{
		"#!/usr/bin/env sh",
		"echo \"src/fail.ts(1,7): error TS2322: Type 'string' is not assignable to type 'number'.\"",
		"echo \"src/fail2.ts(2,10): error TS2304: Cannot find name 'missingSymbol'.\"",
		"exit 2",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "--noEmit", "-p", "tsconfig.json"})
	})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(out, "src/fail.ts:") || !strings.Contains(out, "TS2322") {
		t.Fatalf("expected grouped tsc output, got %q", out)
	}
	if !strings.Contains(out, "src/fail2.ts:") || !strings.Contains(out, "TS2304") {
		t.Fatalf("expected second grouped file, got %q", out)
	}
}

func TestRunnerTscUnsupportedShapeFallsBackToPassthrough(t *testing.T) {
	skipWindowsTscFixture(t)
	r, script := newTscFixtureRunner(t, Options{}, []string{
		"#!/usr/bin/env sh",
		"echo 'watch mode passthrough output'",
		"exit 7",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "--watch"})
	})
	if code != 7 {
		t.Fatalf("expected exit code 7, got %d", code)
	}
	if !strings.Contains(out, "watch mode passthrough output") {
		t.Fatalf("expected passthrough output, got %q", out)
	}
}

func skipWindowsTscFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-specific")
	}
}

func newTscFixtureRunner(t *testing.T, opts Options, scriptLines []string) (*Runner, string) {
	t.Helper()
	work := t.TempDir()
	script := filepath.Join(work, "tsc")
	content := strings.Join(scriptLines, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewTscFilter()); err != nil {
		t.Fatalf("register tsc filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(opts, eng, reg), script
}
