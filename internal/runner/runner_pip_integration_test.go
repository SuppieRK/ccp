package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

const (
	errUnixPIPFixtureMsg = "shell script fixture is unix-specific"
	pipShebangLine       = "#!/usr/bin/env sh"
	pipUnexpectedInvoke  = "echo 'unexpected invocation' 1>&2"
	pipExitThreeLine     = "exit 3"
	errRegisterPIPIntFmt = "register pip filter: %v"
	errWritePIPScriptFmt = "write pip script: %v"
	sameLineValue        = "same-line"
	exitZeroTail         = "\nexit 0\n"
)

func TestRunnerPIPNonZeroExitParityAndStderrPassthroughInstall(t *testing.T) {
	skipWindowsPIPFixture(t)
	work := t.TempDir()
	script := filepath.Join(work, "pip")
	content := strings.Join([]string{
		pipShebangLine,
		"if [ \"$1\" = \"install\" ]; then",
		"  echo 'ERROR: Could not find a version that satisfies the requirement missing' 1>&2",
		"  exit 2",
		"fi",
		pipUnexpectedInvoke,
		pipExitThreeLine,
	}, "\n") + "\n"
	writePIPFixtureScript(t, script, content)

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewPIPFilter()); err != nil {
		t.Fatalf(errRegisterPIPIntFmt, err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	r := New(Options{}, eng, reg)

	withLookPathNotFound(t)

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "install", "missing"})
	})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(out, "Could not find a version") {
		t.Fatalf("expected stderr diagnostic retained, got %q", out)
	}
}

func TestRunnerPIPRawBypass(t *testing.T) {
	skipWindowsPIPFixture(t)
	work := t.TempDir()
	script := filepath.Join(work, "pip")
	content := strings.Join([]string{
		pipShebangLine,
		"if [ \"$1\" = \"list\" ]; then",
		"  echo '[{\"name\":\"requests\",\"version\":\"2.31.0\"}]'",
		"  exit 0",
		"fi",
		pipUnexpectedInvoke,
		pipExitThreeLine,
	}, "\n") + "\n"
	writePIPFixtureScript(t, script, content)

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewPIPFilter()); err != nil {
		t.Fatalf(errRegisterPIPIntFmt, err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	r := New(Options{Raw: true}, eng, reg)

	withLookPathNotFound(t)

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "list"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "[{\"name\":\"requests\",\"version\":\"2.31.0\"}]") {
		t.Fatalf("expected raw output, got %q", out)
	}
}

func TestRunnerPIPContextIsolationFromNpmPnpmYarnPython(t *testing.T) {
	skipWindowsPIPFixture(t)
	work := t.TempDir()
	pipScript := filepath.Join(work, "pip")
	npmScript := filepath.Join(work, "npm")
	pnpmScript := filepath.Join(work, "pnpm")
	yarnScript := filepath.Join(work, "yarn")
	pythonScript := filepath.Join(work, "python")

	writePIPFixtureScript(t, pipScript, pipShebangLine+"\necho '[{\"name\":\"same\",\"version\":\"1.0.0\"}]'"+exitZeroTail)
	writePIPFixtureScript(t, npmScript, pipShebangLine+"\necho '> app@1.0.0 test'\necho '"+sameLineValue+"'"+exitZeroTail)
	writePIPFixtureScript(t, pnpmScript, pipShebangLine+"\necho 'Progress: resolved 10'\necho '"+sameLineValue+"'"+exitZeroTail)
	writePIPFixtureScript(t, yarnScript, pipShebangLine+"\necho '"+sameLineValue+"'"+exitZeroTail)
	writePIPFixtureScript(t, pythonScript, pipShebangLine+"\necho '"+sameLineValue+"'"+exitZeroTail)

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewPIPFilter()); err != nil {
		t.Fatalf(errRegisterPIPIntFmt, err)
	}
	if err := reg.Register(filters.NewNPMFilter()); err != nil {
		t.Fatalf("register npm filter: %v", err)
	}
	if err := reg.Register(filters.NewPNPMFilter()); err != nil {
		t.Fatalf("register pnpm filter: %v", err)
	}
	if err := reg.Register(filters.NewYarnFilter()); err != nil {
		t.Fatalf("register yarn filter: %v", err)
	}
	if err := reg.Register(filters.NewPythonFilter()); err != nil {
		t.Fatalf("register python filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	r := New(Options{}, eng, reg)

	withLookPathNotFound(t)

	assertRunnerOutputContains(t, r, []string{pipScript, "list"}, "pip list:", "pip")
	assertRunnerOutputContains(t, r, []string{npmScript, "run", "test"}, sameLineValue, "npm")
	assertRunnerOutputContains(t, r, []string{pnpmScript, "list"}, sameLineValue, "pnpm")
	assertRunnerOutputContains(t, r, []string{yarnScript, "test"}, sameLineValue, "yarn")
	assertRunnerOutputContains(t, r, []string{pythonScript, "app.py"}, sameLineValue, "python")
}

func assertRunnerOutputContains(t *testing.T, r *Runner, args []string, wantContains string, label string) {
	t.Helper()
	out, code := captureCombined(t, func() int { return r.Run(args) })
	if code != 0 || !strings.Contains(out, wantContains) {
		t.Fatalf("unexpected %s run output=%q code=%d", label, out, code)
	}
}

func TestRunnerPIPSubstitutedUVPathPreservesStderrAndExitParity(t *testing.T) {
	skipWindowsPIPFixture(t)
	work := t.TempDir()
	pipScript := filepath.Join(work, "pip")
	uvScript := filepath.Join(work, "uv")
	writePIPFixtureScript(t, pipScript, pipShebangLine+"\necho 'pip fallback should not run'"+exitZeroTail)
	uvContent := strings.Join([]string{
		pipShebangLine,
		"if [ \"$1\" = \"pip\" ] && [ \"$2\" = \"list\" ]; then",
		"  echo '[{\"name\":\"requests\",\"version\":\"2.31.0\"}]'",
		"  echo 'uv backend warning' 1>&2",
		"  exit 7",
		"fi",
		"echo 'unexpected uv invocation' 1>&2",
		"exit 9",
	}, "\n") + "\n"
	if err := os.WriteFile(uvScript, []byte(uvContent), 0o755); err != nil {
		t.Fatalf("write uv script: %v", err)
	}

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewPIPFilter()); err != nil {
		t.Fatalf(errRegisterPIPIntFmt, err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	r := New(Options{}, eng, reg)

	old := lookPathFn
	defer func() { lookPathFn = old }()
	lookPathFn = func(file string) (string, error) {
		if file == "uv" {
			return uvScript, nil
		}
		return "", exec.ErrNotFound
	}
	oldPath := os.Getenv("PATH")
	defer func() { _ = os.Setenv("PATH", oldPath) }()
	_ = os.Setenv("PATH", work+":"+oldPath)
	resetLookPathCacheForTest()

	out, code := captureCombined(t, func() int {
		return r.Run([]string{pipScript, "list"})
	})
	if code != 7 {
		t.Fatalf("expected substituted uv exit code 7, got %d", code)
	}
	if !strings.Contains(out, "pip list: 1 packages") {
		t.Fatalf("expected compacted list output from substituted uv path, got %q", out)
	}
	if !strings.Contains(out, "uv backend warning") {
		t.Fatalf("expected substituted stderr retained, got %q", out)
	}
}

func TestRunnerPIPFallbackPathParityWhenUVUnavailable(t *testing.T) {
	skipWindowsPIPFixture(t)
	work := t.TempDir()
	pipScript := filepath.Join(work, "pip")
	content := strings.Join([]string{
		pipShebangLine,
		"if [ \"$1\" = \"list\" ]; then",
		"  echo '[{\"name\":\"fallback\",\"version\":\"1.0.0\"}]'",
		"  echo 'fallback stderr' 1>&2",
		"  exit 5",
		"fi",
		pipUnexpectedInvoke,
		pipExitThreeLine,
	}, "\n") + "\n"
	writePIPFixtureScript(t, pipScript, content)

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewPIPFilter()); err != nil {
		t.Fatalf(errRegisterPIPIntFmt, err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	r := New(Options{}, eng, reg)

	withLookPathNotFound(t)

	out, code := captureCombined(t, func() int {
		return r.Run([]string{pipScript, "list"})
	})
	if code != 5 {
		t.Fatalf("expected fallback exit code 5, got %d", code)
	}
	if !strings.Contains(out, "pip list: 1 packages") {
		t.Fatalf("expected fallback compacted output retained, got %q", out)
	}
	if !strings.Contains(out, "fallback stderr") {
		t.Fatalf("expected fallback stderr retained, got %q", out)
	}
}

func skipWindowsPIPFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip(errUnixPIPFixtureMsg)
	}
}

func writePIPFixtureScript(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf(errWritePIPScriptFmt, err)
	}
}

func withLookPathNotFound(t *testing.T) {
	t.Helper()
	old := lookPathFn
	t.Cleanup(func() { lookPathFn = old })
	lookPathFn = func(file string) (string, error) { return "", exec.ErrNotFound }
	resetLookPathCacheForTest()
}
