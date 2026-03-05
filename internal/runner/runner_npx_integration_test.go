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

func TestRunnerNPXRoutedOutputAndExitParity(t *testing.T) {
	skipWindowsNPXFixture(t)
	r, script := newNPXFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"echo 'Need to install the following packages:'",
		"echo '  tsc@5.7.0'",
		"echo 'Ok to proceed? (y)'",
		"echo 'src/app.ts:1:1 - error TS2304: Cannot find name x'",
		"echo 'npm WARN exec install noise'",
		"exit 2",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "tsc", "--noEmit"})
	})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	for _, forbidden := range []string{"Need to install", "Ok to proceed"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("expected wrapper noise removed, got %q", out)
		}
	}
	if !strings.Contains(out, "error TS2304") {
		t.Fatalf("expected payload retained, got %q", out)
	}
}

func TestRunnerNPXPackageFlagPassthroughExitParity(t *testing.T) {
	skipWindowsNPXFixture(t)
	r, script := newNPXFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"echo 'raw passthrough output for -p mode'",
		"exit 7",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "-p", "cowsay", "lolcat"})
	})
	if code != 7 {
		t.Fatalf("expected exit code 7, got %d", code)
	}
	if !strings.Contains(out, "raw passthrough output") {
		t.Fatalf("expected raw passthrough output, got %q", out)
	}
}

func skipWindowsNPXFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-specific")
	}
}

func newNPXFixtureRunner(t *testing.T, scriptLines []string) (*Runner, string) {
	t.Helper()
	work := t.TempDir()
	script := filepath.Join(work, "npx")
	content := strings.Join(scriptLines, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewNPXFilter()); err != nil {
		t.Fatalf("register npx filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(Options{}, eng, reg), script
}
