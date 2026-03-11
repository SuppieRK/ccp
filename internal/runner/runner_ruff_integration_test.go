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

func TestRunnerRuffCompactsSupportedOutputAndPreservesExitCode(t *testing.T) {
	skipWindowsRuffFixture(t)
	r, script := newRuffFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"printf '%s\n' '['",
		"printf '%s\n' '{\"code\":\"F401\",\"message\":\"unused import\",\"location\":{\"row\":1,\"column\":8},\"filename\":\"src/app.py\",\"fix\":{\"applicability\":\"safe\"}},'",
		"printf '%s\n' '{\"code\":\"E501\",\"message\":\"line too long\",\"location\":{\"row\":10,\"column\":89},\"filename\":\"src/utils.py\",\"fix\":null}'",
		"printf '%s\n' ']'",
		"exit 1",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "src"})
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	for _, want := range []string{"ruff: 2 issues in 2 files", "F401", "src/app.py", "10:89 E501"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in %q", want, out)
		}
	}
}

func TestRunnerRuffStructuredModePassthrough(t *testing.T) {
	skipWindowsRuffFixture(t)
	r, script := newRuffFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"echo '[{\"code\":\"F401\"}]'",
		"exit 0",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "check", "--output-format", "json", "src"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "[{\"code\":\"F401\"}]") {
		t.Fatalf("expected passthrough output, got %q", out)
	}
}

func skipWindowsRuffFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-specific")
	}
}

func newRuffFixtureRunner(t *testing.T, scriptLines []string) (*Runner, string) {
	t.Helper()
	work := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(work); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	script := filepath.Join(work, "ruff")
	content := strings.Join(scriptLines, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewRuffFilter()); err != nil {
		t.Fatalf("register ruff filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(Options{}, eng, reg), "./ruff"
}
