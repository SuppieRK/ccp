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
	errUnixFixtureOnlyMsg = "shell script fixture is unix-specific"
	shellScriptShebang    = "#!/usr/bin/env sh"
	unexpectedInvokeMsg   = "echo 'unexpected invocation' 1>&2"
	exitThreeLine         = "exit 3"
	errWriteScriptFmt     = "write script: %v"
	errRegisterCargoFmt   = "register cargo filter: %v"
)

func TestRunnerCargoTestCompaction(t *testing.T) {
	skipWindowsCargoFixture(t)
	r, script := newCargoFixtureRunner(t, Options{}, []string{
		shellScriptShebang,
		"if [ \"$1\" = \"test\" ]; then",
		"  echo 'Running unittests src/lib.rs (target/debug/deps/app-abc)'",
		"  echo 'test result: ok. 2 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out'",
		"  echo 'Doc-tests app'",
		"  echo 'test result: ok. 1 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out'",
		"  exit 0",
		"fi",
		unexpectedInvokeMsg,
		exitThreeLine,
	})

	out, code := captureCombined(t, func() int { return r.Run([]string{script, "test", "./..."}) })
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, "cargo test: ok") {
		t.Fatalf("expected cargo test summary, got %q", out)
	}
}

func TestRunnerCargoStderrVisibilityAndExitParity(t *testing.T) {
	skipWindowsCargoFixture(t)
	r, script := newCargoFixtureRunner(t, Options{}, []string{
		shellScriptShebang,
		"if [ \"$1\" = \"build\" ]; then",
		"  echo 'error[E0425]: cannot find value x in this scope' 1>&2",
		"  exit 9",
		"fi",
		unexpectedInvokeMsg,
		exitThreeLine,
	})

	out, code := captureCombined(t, func() int { return r.Run([]string{script, "build"}) })
	if code != 9 {
		t.Fatalf("expected exit 9, got %d", code)
	}
	if !strings.Contains(out, "error[E0425]") {
		t.Fatalf("expected stderr retained, got %q", out)
	}
}

func TestRunnerCargoRawBypass(t *testing.T) {
	skipWindowsCargoFixture(t)
	r, script := newCargoFixtureRunner(t, Options{Raw: true}, []string{
		shellScriptShebang,
		"if [ \"$1\" = \"check\" ]; then",
		"  echo 'Fresh serde v1.0.214'",
		"  echo 'Finished dev [unoptimized + debuginfo] target(s) in 0.12s'",
		"  exit 0",
		"fi",
		unexpectedInvokeMsg,
		exitThreeLine,
	})

	out, code := captureCombined(t, func() int { return r.Run([]string{script, "check"}) })
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if strings.Contains(out, "cargo check: ok") {
		t.Fatalf("expected raw output in raw mode, got %q", out)
	}
	if !strings.Contains(out, "Fresh serde") {
		t.Fatalf("expected native output, got %q", out)
	}
}

func TestRunnerCargoFilteredStripsANSI(t *testing.T) {
	skipWindowsCargoFixture(t)
	r, script := newCargoFixtureRunner(t, Options{}, []string{
		shellScriptShebang,
		"if [ \"$1\" = \"build\" ]; then",
		"  printf '\\033[31merror[E0425]: cannot find value x in this scope\\033[0m\\n' 1>&2",
		"  exit 1",
		"fi",
		unexpectedInvokeMsg,
		exitThreeLine,
	})

	out, code := captureCombined(t, func() int { return r.Run([]string{script, "build"}) })
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI stripped in filtered mode, got %q", out)
	}
	if !strings.Contains(out, "error[E0425]") {
		t.Fatalf("expected diagnostic visibility after stripping ANSI, got %q", out)
	}
}

func skipWindowsCargoFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip(errUnixFixtureOnlyMsg)
	}
}

func newCargoFixtureRunner(t *testing.T, opts Options, scriptLines []string) (*Runner, string) {
	t.Helper()
	work := t.TempDir()
	script := filepath.Join(work, "cargo")
	content := strings.Join(scriptLines, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf(errWriteScriptFmt, err)
	}

	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewCargoToolFilter()); err != nil {
		t.Fatalf(errRegisterCargoFmt, err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	return New(opts, eng, reg), script
}
