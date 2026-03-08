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

func TestRunnerGitShowCompactsDefaultOutput(t *testing.T) {
	skipWindowsGitFixture(t)
	r, script := newGitFixtureRunner(t, Options{}, []string{
		"#!/usr/bin/env sh",
		"cat <<'EOF'",
		"commit 05377dbf15f3ab2b35bef3df0d7d47c58da6d688",
		"Author: bench <bench@example.com>",
		"Date:   Sun Mar 8 08:17:08 2026 +0100",
		"",
		"    third commit",
		"",
		"diff --git a/tracked.txt b/tracked.txt",
		"index 8c1384d..29ef827 100644",
		"--- a/tracked.txt",
		"+++ b/tracked.txt",
		"@@ -1 +1 @@",
		"-v2",
		"+v3",
		"EOF",
		"exit 0",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "show", "HEAD"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	for _, want := range []string{
		"commit 05377dbf15f3",
		"subject: third commit",
		"tracked.txt  +1 -1",
		"summary: 1 files changed, +1 -1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got %q", want, out)
		}
	}
	if strings.Contains(out, "diff --git") {
		t.Fatalf("expected diff body to be compacted, got %q", out)
	}
}

func TestRunnerGitShowPrecisionSensitivePassthroughAndExitParity(t *testing.T) {
	skipWindowsGitFixture(t)
	r, script := newGitFixtureRunner(t, Options{}, []string{
		"#!/usr/bin/env sh",
		"echo '05377dbf15f3ab2b35bef3df0d7d47c58da6d688'",
		"echo 'fatal: bad revision' 1>&2",
		"exit 7",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "show", "--format=%H"})
	})
	if code != 7 {
		t.Fatalf("expected exit code 7, got %d", code)
	}
	for _, want := range []string{"05377dbf15f3ab2b35bef3df0d7d47c58da6d688", "fatal: bad revision"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in passthrough output, got %q", want, out)
		}
	}
}

func skipWindowsGitFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-specific")
	}
}

func newGitFixtureRunner(t *testing.T, opts Options, scriptLines []string) (*Runner, string) {
	t.Helper()
	work := t.TempDir()
	script := filepath.Join(work, "git")
	content := strings.Join(scriptLines, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewGitToolFilter()); err != nil {
		t.Fatalf("register git filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(opts, eng, reg), script
}
