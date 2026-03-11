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

func TestRunnerPlaywrightCompactsSupportedOutputAndPreservesExitCode(t *testing.T) {
	skipWindowsPlaywrightFixture(t)
	r, script := newPlaywrightFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"echo 'Running 3 tests using 1 worker'",
		"echo '  ✓  1 tests/auth.spec.ts:3:1 › auth › logs in (1.1s)'",
		"echo '  ✘  2 tests/auth.spec.ts:8:1 › auth › rejects bad password (2.0s)'",
		"echo '    Error: expect(received).toBeTruthy()'",
		"echo '  2 passed, 1 failed (3.1s)'",
		"exit 1",
	})

	out, code := captureCombined(t, func() int { return r.Run([]string{script, "test"}) })
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	for _, want := range []string{"playwright: 2 passed, 1 failed (3.1s)", "failed tests:", "tests/auth.spec.ts:8:1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestRunnerPlaywrightReporterModePassthrough(t *testing.T) {
	skipWindowsPlaywrightFixture(t)
	r, script := newPlaywrightFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"echo '{\"reporter\":\"json\"}'",
		"exit 0",
	})

	out, code := captureCombined(t, func() int { return r.Run([]string{script, "test", "--reporter=json"}) })
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "{\"reporter\":\"json\"}") {
		t.Fatalf("expected passthrough output, got %q", out)
	}
}

func skipWindowsPlaywrightFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-specific")
	}
}

func newPlaywrightFixtureRunner(t *testing.T, scriptLines []string) (*Runner, string) {
	t.Helper()
	work := t.TempDir()
	script := filepath.Join(work, "playwright")
	content := strings.Join(scriptLines, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewPlaywrightFilter()); err != nil {
		t.Fatalf("register playwright filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(Options{}, eng, reg), script
}
