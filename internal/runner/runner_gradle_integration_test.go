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

func TestRunnerGradleSharedContextAcrossStdoutStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-specific")
	}
	work := t.TempDir()
	script := filepath.Join(work, "gradlew")
	content := strings.Join([]string{
		"#!/usr/bin/env sh",
		"echo '> Task :app:test FAILED'",
		"echo 'FAILURE: Build failed with an exception.' 1>&2",
		"echo 'Caused by: java.lang.RuntimeException: boom' 1>&2",
		"echo '* Get more help at https://help.gradle.org' 1>&2",
	}, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewGradleFilter()); err != nil {
		t.Fatalf("register gradle filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: registry, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	r := New(Options{}, eng, registry)

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "test"})
	})
	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}
	if !strings.Contains(out, "FAILURE: Build failed with an exception.") {
		t.Fatalf("expected failure marker retained, got %q", out)
	}
	if !strings.Contains(out, "Caused by:") {
		t.Fatalf("expected caused-by retained, got %q", out)
	}
}
