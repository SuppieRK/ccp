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
	errUnixDockerFixtureMsg = "shell script fixture is unix-specific"
	dockerShebangLine       = "#!/usr/bin/env sh"
	dockerUnexpectedInvoke  = "echo 'unexpected invocation' 1>&2"
	dockerExitThreeLine     = "exit 3"
	errWriteDockerScriptFmt = "write script: %v"
	errRegisterDockerFmt    = "register docker filter: %v"
)

func TestRunnerDockerPSCompactionAndExitParity(t *testing.T) {
	skipWindowsDockerFixture(t)
	r, script := newDockerFixtureRunner(t, Options{}, dockerPSScriptLines())

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "ps"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "docker ps: 2 containers") {
		t.Fatalf("expected compact summary, got %q", out)
	}
	if !strings.Contains(out, "[ok x2] nginx") {
		t.Fatalf("expected folded healthy rows, got %q", out)
	}
}

func TestRunnerDockerStderrVisibilityAndNonZeroExitParity(t *testing.T) {
	skipWindowsDockerFixture(t)
	r, script := newDockerFixtureRunner(t, Options{}, []string{
		dockerShebangLine,
		"if [ \"$1\" = \"logs\" ]; then",
		"  echo 'daemon unreachable' 1>&2",
		"  exit 11",
		"fi",
		dockerUnexpectedInvoke,
		dockerExitThreeLine,
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "logs", "api"})
	})
	if code != 11 {
		t.Fatalf("expected exit code 11, got %d", code)
	}
	if !strings.Contains(out, "daemon unreachable") {
		t.Fatalf("expected stderr diagnostic retained, got %q", out)
	}
}

func TestRunnerDockerRawBypass(t *testing.T) {
	skipWindowsDockerFixture(t)
	r, script := newDockerFixtureRunner(t, Options{Raw: true}, dockerPSScriptLines())

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "ps"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.Contains(out, "docker ps: ") {
		t.Fatalf("expected raw-mode bypass, got %q", out)
	}
	if !strings.Contains(out, "CONTAINER ID") || !strings.Contains(out, "web-2") {
		t.Fatalf("expected raw docker output, got %q", out)
	}
}

func skipWindowsDockerFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip(errUnixDockerFixtureMsg)
	}
}

func dockerPSScriptLines() []string {
	return []string{
		dockerShebangLine,
		"if [ \"$1\" = \"ps\" ]; then",
		"  echo 'CONTAINER ID   IMAGE   COMMAND   CREATED   STATUS   PORTS   NAMES'",
		"  echo 'a1            nginx   \"ng\"     now       Up 1m    0.0.0.0:80->80/tcp   web-1'",
		"  echo 'b2            nginx   \"ng\"     now       Up 1m    :::80->80/tcp        web-2'",
		"  exit 0",
		"fi",
		dockerUnexpectedInvoke,
		dockerExitThreeLine,
	}
}

func newDockerFixtureRunner(t *testing.T, opts Options, scriptLines []string) (*Runner, string) {
	t.Helper()
	work := t.TempDir()
	script := filepath.Join(work, "docker")
	content := strings.Join(scriptLines, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf(errWriteDockerScriptFmt, err)
	}

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewDockerToolFilter()); err != nil {
		t.Fatalf(errRegisterDockerFmt, err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(opts, eng, reg), script
}
