package runner

import (
	"io"
	"os"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const windowsCmdExe = "cmd.exe"

func TestRunnerEngineSharedContextAcrossStdoutStderr(t *testing.T) {
	r := newSharedContextRunner()

	out, code := captureCombined(t, func() int {
		if isWindows() {
			return r.Run([]string{windowsCmdExe, "/C", "(echo same-line&echo same-line 1>&2)"})
		}
		return r.Run([]string{"sh", "-c", "echo same-line; echo same-line 1>&2"})
	})
	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}
	if strings.Count(out, "same-line") != 1 {
		t.Fatalf("expected deduped shared-context output once, got %q", out)
	}
}

func TestRunnerEngineChainedCommandsDoNotLeakStateAcrossRuns(t *testing.T) {
	r := newSharedContextRunner()

	first, code := captureStdout(t, func() int {
		if isWindows() {
			return r.Run([]string{windowsCmdExe, "/C", "echo first"})
		}
		return r.Run([]string{"sh", "-c", "echo first"})
	})
	if code != 0 {
		t.Fatalf("expected first run code 0, got %d", code)
	}
	if !strings.Contains(first, "first") {
		t.Fatalf("expected first output, got %q", first)
	}

	second, code := captureStdout(t, func() int {
		if isWindows() {
			return r.Run([]string{windowsCmdExe, "/C", "echo second && echo second"})
		}
		return r.Run([]string{"sh", "-c", "echo second && echo second"})
	})
	if code != 0 {
		t.Fatalf("expected second run code 0, got %d", code)
	}
	if strings.Contains(second, "first") {
		t.Fatalf("unexpected state leak from previous run: %q", second)
	}
	if strings.Count(second, "second") != 1 {
		t.Fatalf("expected per-run dedupe to keep one line, got %q", second)
	}
}

func newSharedContextRunner() *Runner {
	eng := engine.NewEngine(engine.Config{
		NeverDropPatterns: nil,
		Filters: []engine.ToolFilter{sharedCollectFilter{
			runnerTestFilterBase: runnerTestFilterBase{
				tool:          "sh",
				aliases:       []string{windowsCmdExe, "cmd"},
				sharedContext: true,
			},
		}},
	})
	return New(Options{}, eng, nil)
}

func captureCombined(t *testing.T, run func() int) (string, int) {
	t.Helper()
	oldOut := os.Stdout
	oldErr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	os.Stderr = w
	done := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()

	code := run()
	if err := w.Close(); err != nil {
		t.Fatalf("close write pipe: %v", err)
	}
	os.Stdout = oldOut
	os.Stderr = oldErr

	out := <-done
	if err := r.Close(); err != nil && !strings.Contains(err.Error(), "file already closed") {
		t.Fatalf("close read pipe: %v", err)
	}
	return string(out), code
}

type sharedCollectFilter struct {
	runnerTestFilterBase
}

func (f sharedCollectFilter) Process(ev engine.Event, _ *engine.OrderedSetBuffer) engine.Decision {
	if ev.Type == engine.EventEOF {
		return engine.Decision{Action: engine.ActionFlush}
	}
	return engine.Decision{Action: engine.ActionCollect}
}
