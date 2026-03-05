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

func TestRunnerPNPMInstallCompactionAndExitParity(t *testing.T) {
	skipWindowsPNPMFixture(t)
	r, script := newPNPMFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"if [ \"$1\" = \"install\" ]; then",
		"  echo 'Progress: resolved 100, reused 1, downloaded 2, added 3'",
		"  echo 'dependencies:'",
		"  echo '+ lodash 4.17.21'",
		"  echo 'Done in 2.0s'",
		"  exit 0",
		"fi",
		"echo 'unexpected invocation' 1>&2",
		"exit 2",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "install", "lodash"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.Contains(out, "Progress:") {
		t.Fatalf("expected progress suppression, got %q", out)
	}
	if !strings.Contains(out, "+ lodash") {
		t.Fatalf("expected retained summary line, got %q", out)
	}
}

func TestRunnerPNPMOutdatedNoChangeMarkerAndFailureParity(t *testing.T) {
	skipWindowsPNPMFixture(t)
	r, script := newPNPMFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"if [ \"$1\" = \"outdated\" ]; then",
		"  if [ \"$2\" = \"--fail\" ]; then",
		"    echo 'ERR_PNPM_OUTDATED unexpected error' 1>&2",
		"    exit 9",
		"  fi",
		"  echo '[]'",
		"  exit 0",
		"fi",
		"echo 'unexpected invocation' 1>&2",
		"exit 2",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "outdated"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "All packages up-to-date") {
		t.Fatalf("expected up-to-date marker, got %q", out)
	}

	out, code = captureCombined(t, func() int {
		return r.Run([]string{script, "outdated", "--fail"})
	})
	if code != 9 {
		t.Fatalf("expected exit code 9, got %d", code)
	}
	if strings.Contains(out, "All packages up-to-date") {
		t.Fatalf("unexpected success marker on failure, got %q", out)
	}
}

func skipWindowsPNPMFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-specific")
	}
}

func newPNPMFixtureRunner(t *testing.T, scriptLines []string) (*Runner, string) {
	t.Helper()
	work := t.TempDir()
	script := filepath.Join(work, "pnpm")
	content := strings.Join(scriptLines, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewPNPMFilter()); err != nil {
		t.Fatalf("register pnpm filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(Options{}, eng, reg), script
}
