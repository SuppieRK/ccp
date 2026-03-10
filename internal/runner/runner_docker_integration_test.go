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
	if strings.Contains(out, "docker ps: ") {
		t.Fatalf("expected no summary line, got %q", out)
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

func TestRunnerDockerComposeLogsFlushAndExitParity(t *testing.T) {
	skipWindowsDockerFixture(t)
	r, script := newDockerFixtureRunner(t, Options{}, []string{
		dockerShebangLine,
		"if [ \"$1\" = \"compose\" ] && [ \"$2\" = \"logs\" ]; then",
		"  echo 'api-1  | started'",
		"  echo 'web-1  | listening on :8080'",
		"  exit 0",
		"fi",
		dockerUnexpectedInvoke,
		dockerExitThreeLine,
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "compose", "logs", "api"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "api-1  | started") || !strings.Contains(out, "web-1  | listening on :8080") {
		t.Fatalf("expected raw compose logs output, got %q", out)
	}
}

func TestRunnerDockerComposePSCompactionAndExitParity(t *testing.T) {
	skipWindowsDockerFixture(t)
	r, script := newDockerFixtureRunner(t, Options{}, []string{
		dockerShebangLine,
		"if [ \"$1\" = \"compose\" ] && [ \"$2\" = \"ps\" ]; then",
		"  echo 'NAME                IMAGE            COMMAND          SERVICE   CREATED         STATUS                     PORTS'",
		"  echo 'demo-api-1          demo-api         \"./api\"         api       10 seconds ago  Up 10 seconds             0.0.0.0:8080->8080/tcp'",
		"  echo 'demo-worker-1       demo-worker      \"./worker\"      worker    10 seconds ago  Exited (1) 1 second ago  -'",
		"  exit 0",
		"fi",
		dockerUnexpectedInvoke,
		dockerExitThreeLine,
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "compose", "ps"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.Contains(out, "docker compose ps:") {
		t.Fatalf("expected no summary line, got %q", out)
	}
	if !strings.Contains(out, "[ok] demo-api-1 service=api") || !strings.Contains(out, "[!] demo-worker-1 service=worker") {
		t.Fatalf("expected compact service rows, got %q", out)
	}
}

func TestRunnerDockerComposePSFormatPassthrough(t *testing.T) {
	skipWindowsDockerFixture(t)
	r, script := newDockerFixtureRunner(t, Options{}, []string{
		dockerShebangLine,
		"if [ \"$1\" = \"compose\" ] && [ \"$2\" = \"ps\" ]; then",
		"  echo '{\"Service\":\"api\",\"State\":\"running\"}'",
		"  exit 0",
		"fi",
		dockerUnexpectedInvoke,
		dockerExitThreeLine,
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "compose", "ps", "--format", "json"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.Contains(out, "docker compose ps:") {
		t.Fatalf("expected passthrough output, got %q", out)
	}
	if !strings.Contains(out, "{\"Service\":\"api\",\"State\":\"running\"}") {
		t.Fatalf("expected raw structured output, got %q", out)
	}
}

func TestRunnerDockerComposeBuildCompactionAndExitParity(t *testing.T) {
	skipWindowsDockerFixture(t)
	r, script := newDockerFixtureRunner(t, Options{}, []string{
		dockerShebangLine,
		"if [ \"$1\" = \"compose\" ] && [ \"$2\" = \"build\" ]; then",
		"  echo 'project-api  Built'",
		"  echo 'project-web  Built'",
		"  echo '#2 [web internal] load build definition from Dockerfile           0.0s'",
		"  echo '#8 [api 1/4] FROM docker.io/library/golang:1.26-alpine           0.0s'",
		"  echo '#9 [web 1/4] FROM docker.io/library/node:20-alpine               0.0s'",
		"  echo '#18 [api] exporting to image                                     0.0s'",
		"  echo '#19 [web] exporting to image                                     0.0s'",
		"  exit 0",
		"fi",
		dockerUnexpectedInvoke,
		dockerExitThreeLine,
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "compose", "build"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.Contains(out, "[+] Building") {
		t.Fatalf("did not expect synthetic build summary, got %q", out)
	}
	if !strings.Contains(out, "[ok] api built") || !strings.Contains(out, "[ok] web built") {
		t.Fatalf("expected per-service build summary, got %q", out)
	}
}

func TestRunnerDockerComposeBuildFlagPassthrough(t *testing.T) {
	skipWindowsDockerFixture(t)
	r, script := newDockerFixtureRunner(t, Options{}, []string{
		dockerShebangLine,
		"if [ \"$1\" = \"compose\" ] && [ \"$2\" = \"build\" ]; then",
		"  echo 'raw build output'",
		"  exit 0",
		"fi",
		dockerUnexpectedInvoke,
		dockerExitThreeLine,
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "compose", "build", "--progress", "plain"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "raw build output") {
		t.Fatalf("expected passthrough output, got %q", out)
	}
}

func TestRunnerDockerComposeLogsFollowPassthrough(t *testing.T) {
	skipWindowsDockerFixture(t)
	r, script := newDockerFixtureRunner(t, Options{}, []string{
		dockerShebangLine,
		"if [ \"$1\" = \"compose\" ] && [ \"$2\" = \"logs\" ]; then",
		"  echo 'api-1  | tail line'",
		"  exit 0",
		"fi",
		dockerUnexpectedInvoke,
		dockerExitThreeLine,
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "compose", "logs", "--follow", "api"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "api-1  | tail line") {
		t.Fatalf("expected passthrough output, got %q", out)
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
