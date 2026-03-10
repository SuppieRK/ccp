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

func TestRunnerPrettierCompactsSupportedOutputAndPreservesExitCode(t *testing.T) {
	skipWindowsPrettierFixture(t)
	r, script := newPrettierFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"echo 'Checking formatting...'",
		"echo '[warn] src/bad.js'",
		"echo '[warn] Code style issues found in the above file. Run Prettier with --write to fix.'",
		"exit 1",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "--check", "src/bad.js"})
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(out, "prettier check: 1 files need formatting") || !strings.Contains(out, "- src/bad.js") {
		t.Fatalf("expected compacted prettier output, got %q", out)
	}
}

func TestRunnerPrettierUnsupportedShapeFallsBackToPassthrough(t *testing.T) {
	skipWindowsPrettierFixture(t)
	r, script := newPrettierFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"echo 'raw passthrough output for unsupported prettier shape'",
		"exit 7",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "--check", "--ignore-unknown", "src/good.js"})
	})
	if code != 7 {
		t.Fatalf("expected exit code 7, got %d", code)
	}
	if !strings.Contains(out, "raw passthrough output") {
		t.Fatalf("expected passthrough output, got %q", out)
	}
}

func skipWindowsPrettierFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-specific")
	}
}

func newPrettierFixtureRunner(t *testing.T, scriptLines []string) (*Runner, string) {
	t.Helper()
	work := t.TempDir()
	script := filepath.Join(work, "prettier")
	content := strings.Join(scriptLines, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewPrettierFilter()); err != nil {
		t.Fatalf("register prettier filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(Options{}, eng, reg), script
}
