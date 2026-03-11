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

func TestRunnerMypyCompactsSupportedOutputAndPreservesExitCode(t *testing.T) {
	skipWindowsMypyFixture(t)
	r, script := newMypyFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"echo 'src/app.py:12: error: Incompatible return value type (got \"str\", expected \"int\")  [return-value]'",
		"echo 'src/app.py:13: note: Consider using Optional[str]'",
		"echo 'src/app.py:18: error: Argument 1 has incompatible type \"int\"; expected \"str\"  [arg-type]'",
		"echo 'Found 2 errors in 1 file (checked 2 source files)'",
		"exit 1",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "src"})
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	for _, want := range []string{
		"mypy: 2 errors in 1 files",
		"- src/app.py (2 errors)",
		"L12 [return-value]",
		"Consider using Optional[str]",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in %q", want, out)
		}
	}
}

func TestRunnerMypyStructuredModePassthrough(t *testing.T) {
	skipWindowsMypyFixture(t)
	r, script := newMypyFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"echo '{\"errors\":[]}'",
		"exit 0",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "--output=json", "src"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "{\"errors\":[]}") {
		t.Fatalf("expected passthrough structured output, got %q", out)
	}
}

func skipWindowsMypyFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-specific")
	}
}

func newMypyFixtureRunner(t *testing.T, scriptLines []string) (*Runner, string) {
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

	script := filepath.Join(work, "mypy")
	content := strings.Join(scriptLines, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewMypyFilter()); err != nil {
		t.Fatalf("register mypy filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(Options{}, eng, reg), "./mypy"
}
