package runner

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

const (
	errUnixYarnFixtureMsg = "shell script fixture is unix-specific"
	errRegisterYarnIntFmt = "register yarn filter: %v"
)

func TestRunnerYarnNonZeroExitParityAndStderrPassthrough(t *testing.T) {
	skipWindowsYarnFixture(t)
	work := t.TempDir()
	script := filepath.Join(work, "yarn")
	content := strings.Join([]string{
		"#!/usr/bin/env sh",
		"echo 'warning on stderr' 1>&2",
		"echo 'Done in 1.20s.'",
		"exit 7",
	}, "\n") + "\n"
	writeYarnFixtureScript(t, script, content)
	r := newYarnRunner(t, Options{})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "install"})
	})
	if code != 7 {
		t.Fatalf("expected exit code 7, got %d", code)
	}
	for _, expected := range []string{"warning on stderr", "Done in 1.20s."} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected passthrough stdout+stderr, got %q", out)
		}
	}
	if strings.Contains(out, "ok") {
		t.Fatalf("unexpected synthetic success marker in yarn output: %q", out)
	}
}

func TestRunnerYarnRawBypass(t *testing.T) {
	skipWindowsYarnFixture(t)
	work := t.TempDir()
	script := filepath.Join(work, "yarn")
	content := strings.Join([]string{
		"#!/usr/bin/env sh",
		"echo 'raw yarn output'",
		"exit 0",
	}, "\n") + "\n"
	writeYarnFixtureScript(t, script, content)
	r := newYarnRunner(t, Options{Raw: true})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "test"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "raw yarn output") {
		t.Fatalf("expected raw passthrough output, got %q", out)
	}
}

func TestRunnerYarnContextIsolationFromNPM(t *testing.T) {
	skipWindowsYarnFixture(t)
	work := t.TempDir()
	yarnScript := filepath.Join(work, "yarn")
	npmScript := filepath.Join(work, "npm")
	writeYarnFixtureScript(t, yarnScript, "#!/usr/bin/env sh\necho 'same-line'\nexit 0\n")
	writeYarnFixtureScript(t, npmScript, "#!/usr/bin/env sh\necho '> app@1.0.0 test'\necho 'same-line'\nexit 0\n")
	r := newYarnRunner(t, Options{}, filters.NewNPMFilter())

	out1, code1 := captureCombined(t, func() int { return r.Run([]string{yarnScript, "test"}) })
	if code1 != 0 || !strings.Contains(out1, "same-line") {
		t.Fatalf("unexpected yarn run output=%q code=%d", out1, code1)
	}
	out2, code2 := captureCombined(t, func() int { return r.Run([]string{npmScript, "run", "test"}) })
	if code2 != 0 || !strings.Contains(out2, "same-line") {
		t.Fatalf("unexpected npm run output=%q code=%d", out2, code2)
	}
}

func TestYarnPassthroughCompatibleWithOversizedLineSafeguard(t *testing.T) {
	long := strings.Repeat("x", maxLineBytes+1) + "\n"
	r := bufio.NewReader(strings.NewReader(long))
	line, overflow, err := readLineBounded(r, maxLineBytes)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if !overflow {
		t.Fatal("expected overflow flag for oversized line")
	}
	if len(line) != maxLineBytes {
		t.Fatalf("expected truncated line length %d, got %d", maxLineBytes, len(line))
	}
}

func TestYarnFilteredModeUsesBufferedFlushOnExit(t *testing.T) {
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewYarnFilter()); err != nil {
		t.Fatalf(errRegisterYarnIntFmt, err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	eng.SetCommandID("yarn sparse")

	out := eng.Process(string(engine.StdoutStream), "yarn", engine.Input{Line: "one-line\n"})
	if out.Ready || !out.Collect {
		t.Fatalf("expected buffered collect for filtered yarn stream, got %#v", out)
	}
	eof := eng.Process(string(engine.StdoutStream), "yarn", engine.Input{EOF: true, Dispatch: "yarn|mode=run"})
	if eof.Ready {
		t.Fatalf("expected eof to defer flush until exit, got %#v", eof)
	}
	exit := eng.Process(string(engine.StdoutStream), "yarn", engine.Input{Exit: true, Code: 0, Dispatch: "yarn|mode=run"})
	if !exit.Ready || strings.TrimSpace(exit.Output) != "one-line" {
		t.Fatalf("expected exit flush with retained payload, got %#v", exit)
	}
}

func BenchmarkYarnReadLineBounded(b *testing.B) {
	payload := bytes.Repeat([]byte("a"), 4096)
	payload = append(payload, '\n')
	for i := 0; i < b.N; i++ {
		r := bufio.NewReader(bytes.NewReader(payload))
		_, _, _ = readLineBounded(r, maxLineBytes)
	}
}

func skipWindowsYarnFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip(errUnixYarnFixtureMsg)
	}
}

func writeYarnFixtureScript(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
}

func newYarnRunner(t *testing.T, opts Options, extraFilters ...engine.ToolFilter) *Runner {
	t.Helper()
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewYarnFilter()); err != nil {
		t.Fatalf(errRegisterYarnIntFmt, err)
	}
	for _, f := range extraFilters {
		if err := reg.Register(f); err != nil {
			t.Fatalf("register extra filter %q: %v", f.Tool(), err)
		}
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(opts, eng, reg)
}
