package runner

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const (
	rawOptLine  = "raw-opt-line\n"
	partialLine = "partial-line"
)

func TestRunnerRunNoCommandProvided(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{name: "default mode"},
		{name: "raw mode", opts: Options{Raw: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(tt.opts, nil, nil)
			stderr, code := captureStderr(t, func() int {
				return r.Run(nil)
			})
			if code != 2 {
				t.Fatalf("expected exit code 2 for empty command, got %d", code)
			}
			if !strings.Contains(stderr, "no command provided") {
				t.Fatalf("expected missing-command diagnostic, got %q", stderr)
			}
		})
	}
}

func TestRunnerRunCommandNotFoundReturns127(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		cmd  string
	}{
		{name: "default mode", cmd: "__ccp_missing_command_for_test__"},
		{name: "raw mode", opts: Options{Raw: true}, cmd: "__ccp_missing_command_for_raw_test__"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(tt.opts, nil, nil)
			_, code := captureStderr(t, func() int {
				return r.Run([]string{tt.cmd})
			})
			if code != 127 {
				t.Fatalf("expected exit code 127 for missing binary, got %d", code)
			}
		})
	}
}

func TestRunnerRunFallsBackWhenPreferredSubstitutionFails(t *testing.T) {
	const preferred = "__ccp_preferred_missing_binary__"
	lookPathMu.Lock()
	prev, hadPrev := lookPathOK[preferred]
	lookPathOK[preferred] = true
	lookPathMu.Unlock()
	restore := func() {
		lookPathMu.Lock()
		if hadPrev {
			lookPathOK[preferred] = prev
		} else {
			delete(lookPathOK, preferred)
		}
		lookPathMu.Unlock()
	}
	defer restore()

	tool := shellToolForTests()
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(edgePreferredSubstitutionFilter{
		tool: tool,
		fallbackArgs: func() []string {
			if isWindows() {
				return []string{"/C", "echo fallback-from-runner"}
			}
			return []string{"-c", "printf '%s\\n' fallback-from-runner"}
		}(),
	}); err != nil {
		t.Fatalf("register filter: %v", err)
	}

	r := New(Options{}, nil, reg)
	stdout, code := captureStdout(t, func() int {
		return r.Run([]string{tool, "unused"})
	})
	if code != 0 {
		t.Fatalf("expected fallback execution success, got %d", code)
	}
	if !strings.Contains(stdout, "fallback-from-runner") {
		t.Fatalf("expected fallback command output, got %q", stdout)
	}
}

func TestRunnerRunCaptureDirErrorReturns1(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{name: "capture raw", opts: Options{CaptureRaw: true}},
		{name: "raw and capture raw", opts: Options{Raw: true, CaptureRaw: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			notDir := tmp + "/capture-file"
			if err := os.WriteFile(notDir, []byte("x"), 0o644); err != nil {
				t.Fatalf("write non-dir path: %v", err)
			}
			tt.opts.CaptureRawDir = notDir

			r := New(tt.opts, nil, nil)
			_, code := captureStderr(t, func() int {
				return r.Run(successCommand())
			})
			if code != 1 {
				t.Fatalf("expected capture setup failure exit code 1, got %d", code)
			}
		})
	}
}

func TestProcessLineRawOptionBypassesEngine(t *testing.T) {
	r := &Runner{opts: Options{Raw: true}}
	stdout, _ := captureStdout(t, func() int {
		got := r.processLine("stdout", "tool", "", rawOptLine, os.Stdout)
		if got != len(rawOptLine) {
			t.Fatalf("processLine bytes = %d, want %d", got, len(rawOptLine))
		}
		return 0
	})
	if stdout != rawOptLine {
		t.Fatalf("unexpected raw option output: %q", stdout)
	}
}

func TestCopyStreamFallsBackOnReaderError(t *testing.T) {
	dst, err := os.CreateTemp(t.TempDir(), "copy-stream")
	if err != nil {
		t.Fatalf("create temp dst: %v", err)
	}
	t.Cleanup(func() { _ = dst.Close() })

	r := &Runner{}
	stats := &streamStats{}
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte(partialLine))
		_ = pw.CloseWithError(errors.New("forced read failure"))
	}()
	r.copyStream("stdout", "tool", "", pr, dst, stats)

	if stats.rawBytes != len(partialLine) {
		t.Fatalf("stats.rawBytes = %d, want %d", stats.rawBytes, len(partialLine))
	}
	if stats.keptBytes != len(partialLine) {
		t.Fatalf("stats.keptBytes = %d, want %d", stats.keptBytes, len(partialLine))
	}
	if _, err := dst.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek dst: %v", err)
	}
	b, err := io.ReadAll(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, partialLine) {
		t.Fatalf("expected fallback payload in dst, got %q", got)
	}
}

func TestSequencedCaptureWriterFlushAndNoOpWrites(t *testing.T) {
	w := &sequencedCaptureWriter{}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush() on empty buffer returned error: %v", err)
	}
	if n, err := w.Write(nil); err != nil || n != 0 {
		t.Fatalf("Write(nil) = (%d, %v), want (0, nil)", n, err)
	}
	if n, err := w.Write([]byte("x")); err != nil || n != 1 {
		t.Fatalf("Write without file/seq = (%d, %v), want (1, nil)", n, err)
	}
}

func TestSequencedCaptureWriterAppliesConfidentialRedactions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create capture file: %v", err)
	}
	t.Cleanup(func() {
		_ = f.Close()
	})

	var seq atomic.Int32
	w := &sequencedCaptureWriter{
		file:         f,
		seq:          &seq,
		confidential: []string{"org.example.secret"},
	}
	if _, err := w.Write([]byte("org.example.secret.value\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close capture file: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read capture file: %v", err)
	}
	if got := string(b); !strings.Contains(got, "***.value") {
		t.Fatalf("expected redacted sequence capture, got %q", got)
	}
}

type edgePreferredSubstitutionFilter struct {
	tool         string
	fallbackArgs []string
}

func (f edgePreferredSubstitutionFilter) Tool() string      { return f.tool }
func (f edgePreferredSubstitutionFilter) Aliases() []string { return nil }
func (f edgePreferredSubstitutionFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{
		NormalizedArgs:        args,
		PreferredSubstitution: "__ccp_preferred_missing_binary__",
		PreferredArgs:         []string{"preferred-should-fail"},
		FallbackArgs:          append([]string{}, f.fallbackArgs...),
	}
}
func (f edgePreferredSubstitutionFilter) ContextKey(engine.Event) string { return "" }
func (f edgePreferredSubstitutionFilter) MaskingHorizon() int            { return 0 }
func (f edgePreferredSubstitutionFilter) Process(engine.Event, *engine.OrderedSetBuffer) engine.Decision {
	return engine.Decision{Action: engine.ActionCollect}
}
