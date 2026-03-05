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

const (
	errUnixPythonFixtureMsg = "shell script fixture is unix-specific"
	pythonShebangLine       = "#!/usr/bin/env sh"
	errWritePythonScriptFmt = "write script: %v"
	errRegisterPythonIntFmt = "register python filter: %v"
	pythonExitZeroLine      = "exit 0"
	pythonSameLine          = "same-line"
)

func TestRunnerPythonNonZeroExitParityAndStderrPassthrough(t *testing.T) {
	skipWindowsPythonFixture(t)
	work := t.TempDir()
	script := filepath.Join(work, "python")
	content := strings.Join([]string{
		pythonShebangLine,
		"echo 'Traceback (most recent call last):' 1>&2",
		"echo '  File \"app.py\", line 1, in <module>' 1>&2",
		"echo 'ValueError: boom' 1>&2",
		"exit 4",
	}, "\n") + "\n"
	writePythonFixtureScript(t, script, content)
	r := newPythonRunner(t, Options{})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "app.py"})
	})
	if code != 4 {
		t.Fatalf("expected exit code 4, got %d", code)
	}
	if !strings.Contains(out, "Traceback (most recent call last):") || !strings.Contains(out, "ValueError: boom") {
		t.Fatalf("expected traceback stderr lines retained, got %q", out)
	}
	if strings.Contains(out, "ok") {
		t.Fatalf("unexpected synthetic success marker in python output: %q", out)
	}
}

func TestRunnerPythonRawBypass(t *testing.T) {
	skipWindowsPythonFixture(t)
	work := t.TempDir()
	script := filepath.Join(work, "python")
	content := strings.Join([]string{
		pythonShebangLine,
		"echo 'raw python output'",
		"echo 'raw python stderr' 1>&2",
		pythonExitZeroLine,
	}, "\n") + "\n"
	writePythonFixtureScript(t, script, content)
	r := newPythonRunner(t, Options{Raw: true})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "script.py"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "raw python output") || !strings.Contains(out, "raw python stderr") {
		t.Fatalf("expected raw passthrough output, got %q", out)
	}
}

func TestRunnerPythonContextIsolationFromNpmPnpmYarn(t *testing.T) {
	skipWindowsPythonFixture(t)
	work := t.TempDir()
	pythonScript := filepath.Join(work, "python")
	npmScript := filepath.Join(work, "npm")
	pnpmScript := filepath.Join(work, "pnpm")
	yarnScript := filepath.Join(work, "yarn")

	writePythonFixtureScript(t, pythonScript, pythonShebangLine+"\necho '"+pythonSameLine+"'\n"+pythonExitZeroLine+"\n")
	writePythonFixtureScript(t, npmScript, pythonShebangLine+"\necho '> app@1.0.0 test'\necho '"+pythonSameLine+"'\n"+pythonExitZeroLine+"\n")
	writePythonFixtureScript(t, pnpmScript, pythonShebangLine+"\necho 'Progress: resolved 10'\necho '"+pythonSameLine+"'\n"+pythonExitZeroLine+"\n")
	writePythonFixtureScript(t, yarnScript, pythonShebangLine+"\necho '"+pythonSameLine+"'\n"+pythonExitZeroLine+"\n")
	r := newPythonRunner(t, Options{}, filters.NewNPMFilter(), filters.NewPNPMFilter(), filters.NewYarnFilter())

	assertPythonRunnerOutputContains(t, r, []string{pythonScript, "script.py"}, pythonSameLine, "python")
	assertPythonRunnerOutputContains(t, r, []string{npmScript, "run", "test"}, pythonSameLine, "npm")
	assertPythonRunnerOutputContains(t, r, []string{pnpmScript, "list"}, pythonSameLine, "pnpm")
	assertPythonRunnerOutputContains(t, r, []string{yarnScript, "test"}, pythonSameLine, "yarn")
}

func assertPythonRunnerOutputContains(t *testing.T, r *Runner, args []string, wantContains string, label string) {
	t.Helper()
	out, code := captureCombined(t, func() int { return r.Run(args) })
	if code != 0 || !strings.Contains(out, wantContains) {
		t.Fatalf("unexpected %s run output=%q code=%d", label, out, code)
	}
}

func TestRunnerPythonFinalTracebackTailVisibleOnCrashExit(t *testing.T) {
	skipWindowsPythonFixture(t)
	work := t.TempDir()
	script := filepath.Join(work, "python")
	content := strings.Join([]string{
		pythonShebangLine,
		"echo 'Traceback (most recent call last):' 1>&2",
		"echo '  File \"app.py\", line 2, in <module>' 1>&2",
		"echo 'RuntimeError: final-line' 1>&2",
		"exit 1",
	}, "\n") + "\n"
	writePythonFixtureScript(t, script, content)
	r := newPythonRunner(t, Options{})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "app.py"})
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(out, "RuntimeError: final-line") {
		t.Fatalf("expected final traceback tail retained, got %q", out)
	}
}

func TestRunnerPytestAndPythonModulePytestEquivalentOutput(t *testing.T) {
	skipWindowsPythonFixture(t)
	work := t.TempDir()
	pytestScript := filepath.Join(work, "pytest")
	pythonScript := filepath.Join(work, "python")

	pytestContent := strings.Join([]string{
		pythonShebangLine,
		"echo '=== 2 passed in 0.03s ==='",
		pythonExitZeroLine,
	}, "\n") + "\n"
	writePythonFixtureScript(t, pytestScript, pytestContent)

	pythonContent := strings.Join([]string{
		pythonShebangLine,
		"if [ \"$1\" = \"-m\" ] && [ \"$2\" = \"pytest\" ]; then",
		"  echo '=== 2 passed in 0.03s ==='",
		"  exit 0",
		"fi",
		"echo 'python passthrough' ",
		pythonExitZeroLine,
	}, "\n") + "\n"
	writePythonFixtureScript(t, pythonScript, pythonContent)
	r := newPythonRunner(t, Options{}, filters.NewPytestFilter())

	outPytest, codePytest := captureCombined(t, func() int {
		return r.Run([]string{pytestScript, "-q"})
	})
	if codePytest != 0 {
		t.Fatalf("expected pytest exit code 0, got %d", codePytest)
	}
	outPythonModule, codePythonModule := captureCombined(t, func() int {
		return r.Run([]string{pythonScript, "-m", "pytest", "-q"})
	})
	if codePythonModule != 0 {
		t.Fatalf("expected python -m pytest exit code 0, got %d", codePythonModule)
	}
	if outPytest != outPythonModule {
		t.Fatalf("expected equivalent output, pytest=%q python-module=%q", outPytest, outPythonModule)
	}
	if strings.TrimSpace(outPytest) != "pytest: 2 passed" {
		t.Fatalf("unexpected compact pytest output: %q", outPytest)
	}
}

func skipWindowsPythonFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip(errUnixPythonFixtureMsg)
	}
}

func writePythonFixtureScript(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf(errWritePythonScriptFmt, err)
	}
}

func newPythonRunner(t *testing.T, options Options, extraFilters ...engine.ToolFilter) *Runner {
	t.Helper()

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewPythonFilter()); err != nil {
		t.Fatalf(errRegisterPythonIntFmt, err)
	}
	for _, f := range extraFilters {
		if err := reg.Register(f); err != nil {
			t.Fatalf("register extra filter %q: %v", f.Tool(), err)
		}
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(options, eng, reg)
}
