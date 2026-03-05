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

func TestRunnerNPMSharedFailureAcrossStdoutStderrAndExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-specific")
	}
	work := t.TempDir()
	script := filepath.Join(work, "npm")
	content := strings.Join([]string{
		"#!/usr/bin/env sh",
		"if [ \"$1\" = \"run\" ]; then",
		"  echo '> app@1.0.0 test'",
		"  echo 'npm ERR! code 1' 1>&2",
		"  echo 'npm ERR! A complete log of this run can be found in:' 1>&2",
		"  echo 'npm ERR!     /tmp/npm-debug.log' 1>&2",
		"  exit 1",
		"fi",
		"echo 'unexpected invocation' 1>&2",
		"exit 2",
	}, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewNPMFilter()); err != nil {
		t.Fatalf("register npm filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	r := New(Options{}, eng, reg)

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "run", "test"})
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(out, "npm ERR! code 1") {
		t.Fatalf("expected npm error retained, got %q", out)
	}
	if strings.Contains(out, "ok") {
		t.Fatalf("unexpected ok marker on failure, got %q", out)
	}
}
