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

const denoUnixFixtureMsg = "shell script fixture is unix-specific"

func TestRunnerDenoCompactionAndExitParity(t *testing.T) {
	skipWindowsDenoFixture(t)
	r, script := newDenoFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"echo 'Download https://deno.land/std/mod.ts'",
		"echo 'Download https://deno.land/std/testing/asserts.ts'",
		"echo 'app payload'",
		"exit 0",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "run", "main.ts"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "[+1 similar progress lines]") {
		t.Fatalf("expected progress fold summary, got %q", out)
	}
	if !strings.Contains(out, "app payload") {
		t.Fatalf("expected payload retained, got %q", out)
	}
}

func TestRunnerDenoStructuredPassthroughExitParity(t *testing.T) {
	skipWindowsDenoFixture(t)
	r, script := newDenoFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"echo '{\"kind\":\"test\",\"ok\":true}'",
		"exit 4",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "test", "--json"})
	})
	if code != 4 {
		t.Fatalf("expected exit code 4, got %d", code)
	}
	if !strings.Contains(out, "{\"kind\":\"test\",\"ok\":true}") {
		t.Fatalf("expected passthrough json output, got %q", out)
	}
}

func skipWindowsDenoFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip(denoUnixFixtureMsg)
	}
}

func newDenoFixtureRunner(t *testing.T, scriptLines []string) (*Runner, string) {
	t.Helper()
	work := t.TempDir()
	script := filepath.Join(work, "deno")
	content := strings.Join(scriptLines, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewDenoFilter()); err != nil {
		t.Fatalf("register deno filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(Options{}, eng, reg), script
}
