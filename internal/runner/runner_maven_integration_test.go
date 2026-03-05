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

func TestRunnerMavenSharedContextAcrossStdoutStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-specific")
	}
	work := t.TempDir()
	script := filepath.Join(work, "mvnw")
	content := strings.Join([]string{
		"#!/usr/bin/env sh",
		"echo '[INFO] --- maven-compiler-plugin:3.14.0:compile (default-compile) @ app ---'",
		"echo '[ERROR] Failed to execute goal org.apache.maven.plugins:maven-compiler-plugin:3.14.0:compile (default-compile) on project app: Compilation failure' 1>&2",
		"echo 'Caused by: java.lang.IllegalStateException: boom' 1>&2",
		"echo '[INFO] BUILD FAILURE' 1>&2",
	}, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewMavenFilter()); err != nil {
		t.Fatalf("register maven filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: registry, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	r := New(Options{}, eng, registry)

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "compile"})
	})
	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}
	if !strings.Contains(out, "[x] app : compile") {
		t.Fatalf("expected failed module/goal summary, got %q", out)
	}
	if !strings.Contains(out, "Failed to execute goal") {
		t.Fatalf("expected failure marker retained, got %q", out)
	}
	if !strings.Contains(out, "Caused by:") {
		t.Fatalf("expected caused-by retained, got %q", out)
	}
}
