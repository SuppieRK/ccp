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

func TestRunnerGolangciLintCompactsSupportedOutputAndPreservesExitCode(t *testing.T) {
	skipWindowsGolangciLintFixture(t)
	r, script := newGolangciLintFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"echo '{\"Issues\":[{\"FromLinter\":\"errcheck\",\"Text\":\"ignored error\",\"Pos\":{\"Filename\":\"internal/api/server.go\",\"Line\":14,\"Column\":2}},{\"FromLinter\":\"revive\",\"Text\":\"missing doc\",\"Pos\":{\"Filename\":\"internal/api/server.go\",\"Line\":20,\"Column\":1}}]}'",
		"exit 1",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "run", "./..."})
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	for _, want := range []string{
		"golangci-lint: 2 issues in 1 files",
		"- internal/api/server.go (2 issues)",
		"14:2 errcheck ignored error",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in %q", want, out)
		}
	}
}

func TestRunnerGolangciLintStructuredModePassthrough(t *testing.T) {
	skipWindowsGolangciLintFixture(t)
	r, script := newGolangciLintFixtureRunner(t, []string{
		"#!/usr/bin/env sh",
		"echo '{\"Issues\":[]}'",
		"exit 0",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "run", "--out-format", "json", "./..."})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "{\"Issues\":[]}") {
		t.Fatalf("expected passthrough structured output, got %q", out)
	}
}

func skipWindowsGolangciLintFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-specific")
	}
}

func newGolangciLintFixtureRunner(t *testing.T, scriptLines []string) (*Runner, string) {
	t.Helper()
	work := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(work); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})

	script := filepath.Join(work, "golangci-lint")
	content := strings.Join(scriptLines, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewGolangciLintFilter()); err != nil {
		t.Fatalf("register golangci-lint filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(Options{}, eng, reg), "./golangci-lint"
}
