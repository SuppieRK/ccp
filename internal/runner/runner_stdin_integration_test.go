package runner

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/metrics"
)

func TestRunnerSemanticForwardsPipedStdin(t *testing.T) {
	if isWindows() {
		t.Skip("stdin integration uses unix cat pipeline")
	}
	r := New(Options{}, nil, nil)
	input := "alpha\nbeta\n"

	withPipedStdin(t, input, func() {
		out, code := captureStdout(t, func() int { return r.Run([]string{"cat"}) })
		if code != 0 {
			t.Fatalf("expected semantic stdin command success, got %d", code)
		}
		if normalizeNL(out) != input {
			t.Fatalf("expected piped stdin to be preserved, got %q", out)
		}
	})
}

func TestRunnerRawForwardsPipedStdin(t *testing.T) {
	if isWindows() {
		t.Skip("stdin integration uses unix cat pipeline")
	}
	r := New(Options{Raw: true}, nil, nil)
	input := "raw-a\nraw-b\n"

	withPipedStdin(t, input, func() {
		out, code := captureStdout(t, func() int { return r.Run([]string{"cat"}) })
		if code != 0 {
			t.Fatalf("expected raw stdin command success, got %d", code)
		}
		if normalizeNL(out) != input {
			t.Fatalf("expected raw piped stdin to be preserved, got %q", out)
		}
	})
}

func TestRunnerEmptyPipedStdinRemainsEmpty(t *testing.T) {
	if isWindows() {
		t.Skip("stdin integration uses unix cat pipeline")
	}
	r := New(Options{}, nil, nil)

	withPipedStdin(t, "", func() {
		out, code := captureStdout(t, func() int { return r.Run([]string{"cat"}) })
		if code != 0 {
			t.Fatalf("expected empty stdin command success, got %d", code)
		}
		if out != "" {
			t.Fatalf("expected zero-byte output for empty stdin, got %q", out)
		}
	})
}

func TestStartPlannedCommandFallbackPreservesStdin(t *testing.T) {
	fallback := successCommand()
	plan := engine.ExecPlan{
		Name:         "__ccp_missing_command__",
		Args:         nil,
		FallbackName: fallback[0],
		FallbackArgs: fallback[1:],
	}

	cmd, stdout, stderr, code, err := startPlannedCommand(plan)
	if err != nil {
		t.Fatalf("expected fallback command to start, got err=%v code=%d", err, code)
	}
	defer closePipes(stdout, stderr)

	if cmd.Stdin != os.Stdin {
		t.Fatalf("expected fallback command stdin to use os.Stdin")
	}
	exitCode, waitErr := waitExitCode(cmd)
	if waitErr != nil {
		t.Fatalf("wait fallback command: %v", waitErr)
	}
	if exitCode != 0 {
		t.Fatalf("expected fallback command exit code 0, got %d", exitCode)
	}
}

func TestAmbiguousStdinSensitivePipelineUsesNeutralExecution(t *testing.T) {
	if isWindows() {
		t.Skip("stdin integration uses unix cat pipeline")
	}
	r := New(Options{}, nil, nil)
	input := "permissive\nstdin\n"
	withPipedStdin(t, input, func() {
		out, code := captureStdout(t, func() int {
			return r.Run([]string{"cat", "|", "cat"})
		})
		if code != 0 {
			t.Fatalf("expected permissive ambiguous execution success, got %d", code)
		}
		if normalizeNL(out) != input {
			t.Fatalf("expected permissive ambiguous pipeline output parity, got %q", out)
		}
	})
}

func TestRunnerMetricsDispatchIncludesStdinMode(t *testing.T) {
	if isWindows() {
		t.Skip("stdin integration uses unix cat pipeline")
	}
	metricsPath := filepath.Join(t.TempDir(), "gain.db")
	r := New(Options{MetricsPath: metricsPath}, nil, nil)

	withPipedStdin(t, "metrics\n", func() {
		if code := r.Run([]string{"cat"}); code != 0 {
			t.Fatalf("expected metrics stdin command success, got %d", code)
		}
	})

	history, err := metrics.QueryHistory(metricsPath, metrics.QueryOptions{})
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("expected one history row")
	}
	if !strings.Contains(history[0].DispatchKey, "stdin=pipe") {
		t.Fatalf("expected stdin dispatch marker, got %q", history[0].DispatchKey)
	}
}

func TestDetectStdinModeClassification(t *testing.T) {
	if got := detectStdinMode(nil); got != "none" {
		t.Fatalf("expected nil stdin mode none, got %q", got)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	_ = w.Close()
	if got := detectStdinMode(r); got != "pipe" {
		_ = r.Close()
		t.Fatalf("expected pipe stdin mode, got %q", got)
	}
	_ = r.Close()

	f, err := os.CreateTemp(t.TempDir(), "stdin-file-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.WriteString(f, "data"); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("seek temp file: %v", err)
	}
	if got := detectStdinMode(f); got != "pipe" {
		t.Fatalf("expected file-backed stdin mode pipe, got %q", got)
	}
}

func withPipedStdin(t *testing.T, input string, run func()) {
	t.Helper()
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if _, err := io.WriteString(w, input); err != nil {
		_ = r.Close()
		_ = w.Close()
		t.Fatalf("write stdin pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		_ = r.Close()
		t.Fatalf("close stdin writer: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = old
		_ = r.Close()
	}()
	run()
}

func normalizeNL(v string) string {
	return strings.ReplaceAll(v, "\r\n", "\n")
}
