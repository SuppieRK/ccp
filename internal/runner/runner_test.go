package runner

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
	"go-command-compression-proxy/internal/metrics"
)

const (
	rtExpectedZeroFmt         = "expected 0, got %d"
	rtCmdExe                  = "cmd.exe"
	rtExpectedFilteredZeroFmt = "expected filtered exit code 0, got %d"
	rtSameLine                = "same-line"
	rtExpectedRawZeroFmt      = "expected raw exit code 0, got %d"
	rtGetWDFmt                = "getwd: %v"
	rtChdirTempFmt            = "chdir temp: %v"
	rtStubSHName              = "stub.sh"
	rtCloseReadPipeFmt        = "close read pipe: %v"
)

func TestExitCodeParityZero(t *testing.T) {
	r := New(Options{}, nil, nil)
	code := r.Run(successCommand())
	if code != 0 {
		t.Fatalf(rtExpectedZeroFmt, code)
	}
}

func TestExitCodeParityNonZero(t *testing.T) {
	r := New(Options{}, nil, nil)
	want := 7
	code := r.Run(failingCommand(want))
	if code != want {
		t.Fatalf("expected %d, got %d", want, code)
	}
}

func TestRunnerPropagatesExitEvent(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{
			name: "success",
			want: 0,
		},
		{
			name: "failure",
			want: 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &exitRecordingFilter{runnerTestFilterBase: runnerTestFilterBase{tool: shellToolForTests()}}
			eng := engine.NewEngine(engine.Config{Filters: []engine.ToolFilter{f}})
			r := New(Options{}, eng, nil)
			var code int
			if tt.want == 0 {
				code = r.Run(successCommand())
			} else {
				code = r.Run(failingCommand(tt.want))
			}
			if code != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, code)
			}
			if !f.seenExit {
				t.Fatal("expected exit event to be propagated")
			}
			if f.exitCode != tt.want {
				t.Fatalf("expected propagated exit=%d, got %d", tt.want, f.exitCode)
			}
		})
	}
}

func TestRunnerRawBypassesExitEventPropagation(t *testing.T) {
	f := &exitRecordingFilter{runnerTestFilterBase: runnerTestFilterBase{tool: shellToolForTests()}}
	eng := engine.NewEngine(engine.Config{
		Filters: []engine.ToolFilter{f},
	})
	r := New(Options{Raw: true}, eng, nil)
	_ = r.Run(failingCommand(5))
	if f.seenExit {
		t.Fatal("expected no exit event in raw mode")
	}
}

func TestRawBypassesDedupe(t *testing.T) {
	eng := engine.NewEngine(engine.Config{
		NeverDropPatterns: nil,
		Filters: []engine.ToolFilter{keepAllFilter{
			runnerTestFilterBase: runnerTestFilterBase{tool: "sh", aliases: []string{rtCmdExe, "cmd"}},
		}},
	})

	filteredRunner := New(Options{}, eng, nil)
	filteredOutput, code := captureStdout(t, func() int {
		return filteredRunner.Run(duplicateLineCommand())
	})
	if code != 0 {
		t.Fatalf(rtExpectedFilteredZeroFmt, code)
	}
	if strings.Count(filteredOutput, rtSameLine) != 1 {
		t.Fatalf("expected deduped output once, got %q", filteredOutput)
	}

	rawRunner := New(Options{Raw: true}, eng, nil)
	rawOutput, code := captureStdout(t, func() int {
		return rawRunner.Run(duplicateLineCommand())
	})
	if code != 0 {
		t.Fatalf(rtExpectedRawZeroFmt, code)
	}
	if strings.Count(rawOutput, rtSameLine) != 2 {
		t.Fatalf("expected raw output twice, got %q", rawOutput)
	}
}

func TestRunnerANSIBehaviorByMode(t *testing.T) {
	tests := []struct {
		name     string
		opts     Options
		mustHave []string
		mustNot  []string
	}{
		{
			name:     "filtered strips ansi and keeps payload",
			opts:     Options{},
			mustHave: []string{"green-line"},
			mustNot:  []string{"\x1b["},
		},
		{
			name:     "raw preserves ansi",
			opts:     Options{Raw: true},
			mustHave: []string{"\x1b[32m"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, code := captureStdout(t, func() int {
				return New(tt.opts, nil, nil).Run(ansiLineCommand())
			})
			if code != 0 {
				t.Fatalf("expected exit code 0, got %d", code)
			}
			for _, must := range tt.mustHave {
				if !strings.Contains(out, must) {
					t.Fatalf("expected output to contain %q, got %q", must, out)
				}
			}
			for _, deny := range tt.mustNot {
				if strings.Contains(out, deny) {
					t.Fatalf("expected output to not contain %q, got %q", deny, out)
				}
			}
		})
	}
}

func TestRunnerConfidentialRedactsSemanticStdout(t *testing.T) {
	r := New(Options{Confidential: []string{"hello"}}, nil, nil)
	out, code := captureStdout(t, func() int {
		return r.Run(stdoutOnlyCommand("hello-stdout"))
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.Contains(out, "hello") {
		t.Fatalf("expected stdout to be redacted, got %q", out)
	}
	if !strings.Contains(out, "***-stdout") {
		t.Fatalf("expected redacted stdout output, got %q", out)
	}
}

func TestRunnerConfidentialRedactsSemanticStderr(t *testing.T) {
	r := New(Options{Confidential: []string{"hello"}}, nil, nil)
	out, code := captureStderr(t, func() int {
		return r.Run(stderrOnlyCommand("hello-stderr"))
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.Contains(out, "hello") {
		t.Fatalf("expected stderr to be redacted, got %q", out)
	}
	if !strings.Contains(out, "***-stderr") {
		t.Fatalf("expected redacted stderr output, got %q", out)
	}
}

func TestRunnerConfidentialRedactsRawStdout(t *testing.T) {
	r := New(Options{Raw: true, Confidential: []string{"hello"}}, nil, nil)
	out, code := captureStdout(t, func() int {
		return r.Run(stdoutOnlyCommand("hello-raw"))
	})
	if code != 0 {
		t.Fatalf("expected raw exit code 0, got %d", code)
	}
	if strings.Contains(out, "hello") {
		t.Fatalf("expected raw stdout to be redacted, got %q", out)
	}
	if !strings.Contains(out, "***-raw") {
		t.Fatalf("expected redacted raw stdout, got %q", out)
	}
}

func TestRunnerCaptureRawAndTerminalBothRedactConfidential(t *testing.T) {
	tmp := t.TempDir()
	restoreWorkingDir(t, tmp)

	r := New(Options{Raw: true, CaptureRaw: true, Confidential: []string{"hello"}}, nil, nil)
	out, code := captureStdout(t, func() int {
		return r.Run(stdoutOnlyCommand("hello-capture"))
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.Contains(out, "hello") {
		t.Fatalf("expected terminal output to be redacted, got %q", out)
	}
	matches := mustGlob(t, filepath.Join(tmp, "ccp-capture-*-input-stdout.txt"), "stdout")
	if len(matches) != 1 {
		t.Fatalf("expected one stdout capture file, got %d", len(matches))
	}
	b, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read stdout capture: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "hello") {
		t.Fatalf("expected capture output to be redacted, got %q", got)
	}
	if !strings.Contains(got, "***-capture") {
		t.Fatalf("expected redacted capture output, got %q", got)
	}
}

func TestRunnerCaptureRawWritesStreamFiles(t *testing.T) {
	tmp := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf(rtGetWDFmt, err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf(rtChdirTempFmt, err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	r := New(Options{Raw: true, CaptureRaw: true}, nil, nil)
	code := r.Run(stdoutStderrCommand())
	if code != 0 {
		t.Fatalf(rtExpectedZeroFmt, code)
	}

	matches, err := filepath.Glob(filepath.Join(tmp, "ccp-capture-*-input-stdout.txt"))
	if err != nil {
		t.Fatalf("glob stdout capture: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one stdout capture file, got %d", len(matches))
	}
	stdoutBytes, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read stdout capture: %v", err)
	}
	stdoutText := string(stdoutBytes)
	if !regexp.MustCompile(`\d{5}\|hello-stdout`).MatchString(stdoutText) {
		t.Fatalf("expected stdout payload, got %q", stdoutText)
	}

	matches, err = filepath.Glob(filepath.Join(tmp, "ccp-capture-*-input-stderr.txt"))
	if err != nil {
		t.Fatalf("glob stderr capture: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one stderr capture file, got %d", len(matches))
	}
	stderrBytes, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read stderr capture: %v", err)
	}
	stderrText := string(stderrBytes)
	if !regexp.MustCompile(`\d{5}\|hello-stderr`).MatchString(stderrText) {
		t.Fatalf("expected stderr payload, got %q", stderrText)
	}
}

func TestRunnerCaptureRawSkipsEmptyStreams(t *testing.T) {
	tmp := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf(rtGetWDFmt, err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf(rtChdirTempFmt, err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	r := New(Options{Raw: true, CaptureRaw: true}, nil, nil)
	code := r.Run(successCommand())
	if code != 0 {
		t.Fatalf(rtExpectedZeroFmt, code)
	}

	stdoutMatches := mustGlob(t, filepath.Join(tmp, "ccp-capture-*-input-stdout.txt"), "stdout")
	stderrMatches := mustGlob(t, filepath.Join(tmp, "ccp-capture-*-input-stderr.txt"), "stderr")
	if len(stdoutMatches) != 0 || len(stderrMatches) != 0 {
		t.Fatalf("expected no capture files for empty output, got stdout=%d stderr=%d", len(stdoutMatches), len(stderrMatches))
	}
}

func TestRawBypassesPrepareRewrites(t *testing.T) {
	tmp := t.TempDir()
	restoreWorkingDir(t, tmp)

	if isWindows() {
		assertRawBypassWindows(t, tmp)
		return
	}
	assertRawBypassUnix(t, tmp)
}

func restoreWorkingDir(t *testing.T, dir string) {
	t.Helper()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf(rtGetWDFmt, err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf(rtChdirTempFmt, err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
}

func newRawDockerRunner(t *testing.T) *Runner {
	t.Helper()
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewDockerToolFilter()); err != nil {
		t.Fatalf("register docker filter: %v", err)
	}
	return New(Options{Raw: true}, nil, reg)
}

func assertNoPrepareRewriteInRawMode(t *testing.T, r *Runner, cmd []string) {
	t.Helper()
	out, code := captureStdout(t, func() int { return r.Run(cmd) })
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.Contains(out, "--format") {
		t.Fatalf("expected no prepare rewrite in raw mode, got %q", out)
	}
}

func assertRawBypassWindows(t *testing.T, tmp string) {
	t.Helper()
	stubPath := filepath.Join(tmp, "stub.cmd")
	if err := os.WriteFile(stubPath, []byte("@echo off\r\nfor %%a in (%*) do @echo %%a\r\n"), 0o644); err != nil {
		t.Fatalf("write stub.cmd: %v", err)
	}
	assertNoPrepareRewriteInRawMode(t, newRawDockerRunner(t), []string{stubPath, "images"})
}

func assertRawBypassUnix(t *testing.T, tmp string) {
	t.Helper()
	stubPath := filepath.Join(tmp, rtStubSHName)
	if err := os.WriteFile(stubPath, []byte("#!/bin/sh\nfor arg in \"$@\"; do\n  printf '%s\\n' \"$arg\"\ndone\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", rtStubSHName, err)
	}
	if err := os.Chmod(stubPath, 0o755); err != nil {
		t.Fatalf("chmod stub: %v", err)
	}
	assertNoPrepareRewriteInRawMode(t, newRawDockerRunner(t), []string{stubPath, "images"})
}

func TestRawDisablesMetricsPersistence(t *testing.T) {
	tmp := t.TempDir()
	metricsPath := filepath.Join(tmp, "gain.jsonl")

	rawRunner := New(Options{Raw: true, MetricsPath: metricsPath}, nil, nil)
	if code := rawRunner.Run(successCommand()); code != 0 {
		t.Fatalf(rtExpectedRawZeroFmt, code)
	}
	if _, err := os.Stat(metricsPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no metrics file in raw mode, stat err=%v", err)
	}

	filteredRunner := New(Options{MetricsPath: metricsPath}, nil, nil)
	if code := filteredRunner.Run(successCommand()); code != 0 {
		t.Fatalf(rtExpectedFilteredZeroFmt, code)
	}
	if _, err := os.Stat(metricsPath); err != nil {
		t.Fatalf("expected metrics file for filtered run: %v", err)
	}
}

func TestRunnerPersistsPassthroughAndProxiedMarkers(t *testing.T) {
	tmp := t.TempDir()
	metricsPath := filepath.Join(tmp, "gain.db")
	r := New(Options{MetricsPath: metricsPath}, nil, nil)

	if code := r.Run(successCommand()); code != 0 {
		t.Fatalf("expected proxied-style command success, got %d", code)
	}
	if code := r.Run([]string{"echo", "a", "&&", "echo", "b"}); code != 0 {
		t.Fatalf("expected ambiguous shell-chain success, got %d", code)
	}

	history, err := metrics.QueryHistory(metricsPath, metrics.QueryOptions{})
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	if len(history) < 2 {
		t.Fatalf("expected at least 2 history rows, got %d", len(history))
	}
	var sawPassthrough bool
	var sawProxied bool
	for _, row := range history {
		if row.Passthrough {
			sawPassthrough = true
		} else {
			sawProxied = true
		}
	}
	if !sawPassthrough || !sawProxied {
		t.Fatalf("expected both passthrough and proxied markers, sawPassthrough=%t sawProxied=%t", sawPassthrough, sawProxied)
	}
}

func TestRunnerMetricsIncludeExitOnlyFilterOutput(t *testing.T) {
	const summary = "exit-summary\n"
	f := &exitFlushFilter{runnerTestFilterBase: runnerTestFilterBase{tool: shellToolForTests()}, summary: summary}
	eng := engine.NewEngine(engine.Config{Filters: []engine.ToolFilter{f}})

	metricsPath := filepath.Join(t.TempDir(), "gain.db")
	r := New(Options{MetricsPath: metricsPath}, eng, nil)

	out, code := captureStdout(t, func() int {
		return r.Run(successCommand())
	})
	if code != 0 {
		t.Fatalf("expected success, got %d", code)
	}
	if !strings.Contains(out, summary) {
		t.Fatalf("expected exit summary in output, got %q", out)
	}

	history, err := metrics.QueryHistory(metricsPath, metrics.QueryOptions{})
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("expected one metrics row")
	}
	row := history[0]
	if row.RawBytes != 0 {
		t.Fatalf("expected raw bytes 0 for no stdout/stderr command output, got %d", row.RawBytes)
	}
	if want := int64(len(summary)); row.KeptBytes != want {
		t.Fatalf("expected kept bytes to include exit-only output (%d), got %d", want, row.KeptBytes)
	}
}

func TestRawBypassesRegistryCompletely(t *testing.T) {
	eng := engine.NewEngine(engine.Config{
		NeverDropPatterns: nil,
		Filters: []engine.ToolFilter{panicFilter{
			runnerTestFilterBase: runnerTestFilterBase{tool: "sh", aliases: []string{rtCmdExe, "cmd"}},
		}},
	})

	rawRunner := New(Options{Raw: true}, eng, nil)
	rawOutput, code := captureStdout(t, func() int {
		return rawRunner.Run(duplicateLineCommand())
	})
	if code != 0 {
		t.Fatalf(rtExpectedRawZeroFmt, code)
	}
	if strings.Count(rawOutput, rtSameLine) != 2 {
		t.Fatalf("expected raw output twice, got %q", rawOutput)
	}
}

func TestAmbiguousChainedCommandExecutesPermissively(t *testing.T) {
	r := New(Options{}, nil, nil)
	out, code := captureStdout(t, func() int {
		return r.Run([]string{"echo", "a", "&&", "echo", "b"})
	})
	if code != 0 {
		t.Fatalf("expected permissive execution success, got %d", code)
	}
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") {
		t.Fatalf("expected chained command output, got %q", out)
	}
}

func TestPermissiveAmbiguousUsesNeutralFiltering(t *testing.T) {
	eng := engine.NewEngine(engine.Config{
		NeverDropPatterns: nil,
		Filters: []engine.ToolFilter{keepAllFilter{
			runnerTestFilterBase: runnerTestFilterBase{tool: "echo", aliases: []string{rtCmdExe, "cmd"}},
		}},
	})
	r := New(Options{}, eng, nil)
	out, code := captureStdout(t, func() int {
		return r.Run([]string{"echo", rtSameLine, "&&", "echo", rtSameLine})
	})
	if code != 0 {
		t.Fatalf("expected permissive ambiguous execution success, got %d", code)
	}
	if strings.Count(out, rtSameLine) != 2 {
		t.Fatalf("expected neutral passthrough for ambiguous execution, got %q", out)
	}
}

func TestDirectExecutionStillWorks(t *testing.T) {
	r := New(Options{}, nil, nil)
	out, code := captureStdout(t, func() int {
		if isWindows() {
			return r.Run([]string{rtCmdExe, "/C", "echo ok"})
		}
		return r.Run([]string{"sh", "-c", "echo ok"})
	})
	if code != 0 {
		t.Fatalf("expected direct execution success, got %d", code)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("expected output, got %q", out)
	}
}

func TestLSSemanticCompaction(t *testing.T) {
	if isWindows() {
		t.Skip("ls compaction test requires unix ls output format")
	}
	if _, err := exec.LookPath("ls"); err != nil {
		t.Skip("ls binary not available")
	}

	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "my file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}

	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewLSCompactor()); err != nil {
		t.Fatalf("register ls compactor: %v", err)
	}
	r := New(Options{}, engine.NewEngine(engine.Config{
		NeverDropPatterns: engine.DefaultNeverDropPatterns(),
		Filters:           []engine.ToolFilter{filters.NewLSCompactor()},
	}), registry)
	out, code := captureStdout(t, func() int {
		return r.Run([]string{"ls", tmp})
	})
	if code != 0 {
		t.Fatalf("expected ls code 0, got %d", code)
	}
	if strings.Contains(out, "total ") {
		t.Fatalf("expected compacted output, got raw ls text: %q", out)
	}
	if !strings.Contains(out, "README.md") || !strings.Contains(out, "my file.txt") {
		t.Fatalf("expected compact output to include file names, got %q", out)
	}
}

func TestLSFailurePreservesStderrAndExitCode(t *testing.T) {
	if isWindows() {
		t.Skip("ls failure parity test requires unix ls output format")
	}
	if _, err := exec.LookPath("ls"); err != nil {
		t.Skip("ls binary not available")
	}

	r := New(Options{}, engine.NewEngine(engine.Config{NeverDropPatterns: engine.DefaultNeverDropPatterns()}), nil)
	stderr, code := captureStderr(t, func() int {
		return r.Run([]string{"ls", "/definitely-missing-dir-for-ccp-test"})
	})
	if code == 0 {
		t.Fatal("expected non-zero exit code for missing ls path")
	}
	if strings.TrimSpace(stderr) == "" {
		t.Fatal("expected stderr diagnostics for failing ls command")
	}
	if strings.Contains(stderr, "(empty)") || strings.Contains(stderr, "summary:") {
		t.Fatalf("expected raw ls diagnostics without compaction markers, got %q", stderr)
	}
}

func successCommand() []string {
	if isWindows() {
		return []string{rtCmdExe, "/C", "exit 0"}
	}
	return []string{"sh", "-c", "exit 0"}
}

func failingCommand(code int) []string {
	if isWindows() {
		return []string{rtCmdExe, "/C", "exit " + itoa(code)}
	}
	return []string{"sh", "-c", "exit " + itoa(code)}
}

func duplicateLineCommand() []string {
	if isWindows() {
		return []string{rtCmdExe, "/C", "(echo " + rtSameLine + "&echo " + rtSameLine + ")"}
	}
	return []string{"sh", "-c", "printf '" + rtSameLine + "\\n" + rtSameLine + "\\n'"}
}

func ansiLineCommand() []string {
	if isWindows() {
		return []string{rtCmdExe, "/C", "echo \u001b[32mgreen-line\u001b[0m"}
	}
	return []string{"sh", "-c", "printf '\\033[32mgreen-line\\033[0m\\n'"}
}

func stdoutStderrCommand() []string {
	if isWindows() {
		return []string{rtCmdExe, "/C", "(echo hello-stdout&echo hello-stderr 1>&2)"}
	}
	return []string{"sh", "-c", "printf 'hello-stdout\\n'; printf 'hello-stderr\\n' 1>&2"}
}

func stdoutOnlyCommand(line string) []string {
	if isWindows() {
		return []string{rtCmdExe, "/C", "echo " + line}
	}
	return []string{"sh", "-c", "printf '%s\\n' \"" + line + "\""}
}

func stderrOnlyCommand(line string) []string {
	if isWindows() {
		return []string{rtCmdExe, "/C", "echo " + line + " 1>&2"}
	}
	return []string{"sh", "-c", "printf '%s\\n' \"" + line + "\" 1>&2"}
}

func shellToolForTests() string {
	if isWindows() {
		return rtCmdExe
	}
	return "sh"
}

func captureStdout(t *testing.T, run func() int) (string, int) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	outCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		b, readErr := io.ReadAll(r)
		if readErr != nil {
			errCh <- readErr
			return
		}
		outCh <- string(b)
	}()

	code := run()

	if err := w.Close(); err != nil {
		t.Fatalf("close write pipe: %v", err)
	}
	os.Stdout = old

	select {
	case readErr := <-errCh:
		if err := r.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Fatalf(rtCloseReadPipeFmt, err)
		}
		t.Fatalf("read pipe: %v", readErr)
	case out := <-outCh:
		if err := r.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Fatalf(rtCloseReadPipeFmt, err)
		}
		return out, code
	}
	return "", code
}

func captureStderr(t *testing.T, run func() int) (string, int) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w

	outCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		b, readErr := io.ReadAll(r)
		if readErr != nil {
			errCh <- readErr
			return
		}
		outCh <- string(b)
	}()

	code := run()

	if err := w.Close(); err != nil {
		t.Fatalf("close write pipe: %v", err)
	}
	os.Stderr = old

	select {
	case readErr := <-errCh:
		if err := r.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Fatalf(rtCloseReadPipeFmt, err)
		}
		t.Fatalf("read pipe: %v", readErr)
	case out := <-outCh:
		if err := r.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Fatalf(rtCloseReadPipeFmt, err)
		}
		return out, code
	}
	return "", code
}

func mustGlob(t *testing.T, pattern string, stream string) []string {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob %s capture: %v", stream, err)
	}
	return matches
}

type panicFilter struct {
	runnerTestFilterBase
}

func (p panicFilter) Process(engine.Event, *engine.OrderedSetBuffer) engine.Decision {
	panic("filter should not be called in raw mode")
}

type keepAllFilter struct {
	runnerTestFilterBase
}

func (k keepAllFilter) Process(ev engine.Event, _ *engine.OrderedSetBuffer) engine.Decision {
	if ev.Type == engine.EventEOF {
		return engine.Decision{Action: engine.ActionFlush}
	}
	return engine.Decision{Action: engine.ActionCollect}
}

type exitRecordingFilter struct {
	runnerTestFilterBase
	seenExit bool
	exitCode int
}

type exitFlushFilter struct {
	runnerTestFilterBase
	summary string
}

func (f *exitFlushFilter) Process(ev engine.Event, _ *engine.OrderedSetBuffer) engine.Decision {
	switch ev.Type {
	case engine.EventExit:
		return engine.Decision{Action: engine.ActionFlush, Output: f.summary}
	default:
		return engine.Decision{Action: engine.ActionIgnore}
	}
}

func (f *exitRecordingFilter) Process(ev engine.Event, _ *engine.OrderedSetBuffer) engine.Decision {
	switch ev.Type {
	case engine.EventLine:
		return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
	case engine.EventExit:
		f.seenExit = true
		f.exitCode = ev.ExitCode
		return engine.Decision{Action: engine.ActionIgnore}
	default:
		return engine.Decision{Action: engine.ActionIgnore}
	}
}
