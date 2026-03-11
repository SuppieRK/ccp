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

func TestRunnerNextBuildCompactsSupportedOutputAndPreservesExitCode(t *testing.T) {
	skipWindowsNextBuildFixture(t)
	r, script := newNextBuildFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"echo '▲ Next.js 15.2.0'",
		"echo 'Creating an optimized production build ...'",
		"echo '✓ Compiled successfully'",
		"echo '├ ○ / 1.2 kB 132 kB'",
		"echo '└ ● /dashboard 2.5 kB 156 kB'",
		"echo '✓ Built in 34.2s'",
		"exit 0",
	})

	out, code := captureCombined(t, func() int { return r.Run([]string{script, "build"}) })
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	for _, want := range []string{"next build: success", "routes: 2 total", "dynamic /dashboard 156.0 kB"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestRunnerNextBuildUnsupportedShapeFallsBackToPassthrough(t *testing.T) {
	skipWindowsNextBuildFixture(t)
	r, script := newNextBuildFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"echo 'debug mode output'",
		"exit 3",
	})

	out, code := captureCombined(t, func() int { return r.Run([]string{script, "build", "--debug"}) })
	if code != 3 {
		t.Fatalf("expected exit code 3, got %d", code)
	}
	if !strings.Contains(out, "debug mode output") {
		t.Fatalf("expected passthrough output, got %q", out)
	}
}

func skipWindowsNextBuildFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-specific")
	}
}

func newNextBuildFixtureRunner(t *testing.T, scriptLines []string) (*Runner, string) {
	t.Helper()
	work := t.TempDir()
	script := filepath.Join(work, "next")
	content := strings.Join(scriptLines, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewNextBuildFilter()); err != nil {
		t.Fatalf("register next build filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(Options{}, eng, reg), script
}
