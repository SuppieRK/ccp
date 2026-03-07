package runner

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go-command-compression-proxy/internal/engine"
)

const rawLineFixture = "raw-line\n"

const (
	captureStdoutFileName = "stdout.txt"
	captureStderrFileName = "stderr.txt"
)

type tickFlushFilter struct {
	runnerTestFilterBase
}

func (f tickFlushFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	switch ev.Type {
	case engine.EventLine:
		return engine.Decision{Action: engine.ActionCollect}
	case engine.EventTick:
		if mem.Len() == 0 {
			return engine.Decision{Action: engine.ActionCollect}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: "tick-flush\n"}
	case engine.EventExit:
		return engine.Decision{Action: engine.ActionIgnore}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

type dispatchCaptureFilter struct {
	runnerTestFilterBase
	dispatchKey string
	mu          sync.Mutex
	lineSeen    bool
	eofSeen     bool
	exitSeen    bool
	tickSeen    bool
	lineValue   string
	eofValue    string
	exitValue   string
	tickValue   string
}

func (f *dispatchCaptureFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{
		NormalizedArgs: args,
		DispatchKey:    f.dispatchKey,
	}
}

func (f *dispatchCaptureFilter) Process(ev engine.Event, _ *engine.OrderedSetBuffer) engine.Decision {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch ev.Type {
	case engine.EventLine:
		f.lineSeen = true
		f.lineValue = ev.Dispatch
		return engine.Decision{Action: engine.ActionCollect}
	case engine.EventEOF:
		f.eofSeen = true
		f.eofValue = ev.Dispatch
		return engine.Decision{Action: engine.ActionFlush}
	case engine.EventExit:
		f.exitSeen = true
		f.exitValue = ev.Dispatch
		return engine.Decision{Action: engine.ActionIgnore}
	case engine.EventTick:
		f.tickSeen = true
		f.tickValue = ev.Dispatch
		return engine.Decision{Action: engine.ActionCollect}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

func (f *dispatchCaptureFilter) snapshot() (line, eof, exit, tick bool, lineVal, eofVal, exitVal, tickVal string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lineSeen, f.eofSeen, f.exitSeen, f.tickSeen, f.lineValue, f.eofValue, f.exitValue, f.tickValue
}

func TestRunnerRegistryAccessor(t *testing.T) {
	reg := engine.NewToolFilterRegistry()
	r := New(Options{}, nil, reg)
	if got := r.Registry(); got != reg {
		t.Fatalf("Registry() returned unexpected pointer: %p != %p", got, reg)
	}
}

func TestCaptureRawLineOutputBehavior(t *testing.T) {
	tests := []struct {
		name               string
		confidential       []string
		write              func(r *Runner)
		wantStdoutContains []string
		wantStderrExact    string
	}{
		{
			name:         "writes sequenced stdout and stderr files",
			confidential: nil,
			write: func(r *Runner) {
				r.captureRawLine("stdout", "hello-out\n")
				r.captureRawLine("stderr", "hello-err\n")
			},
			wantStdoutContains: []string{"00000|hello-out\n"},
			wantStderrExact:    "00001|hello-err\n",
		},
		{
			name:         "applies confidential redactions",
			confidential: []string{"com.secret.pkg"},
			write: func(r *Runner) {
				r.captureRawLine("stdout", "import com.secret.pkg.feature;\n")
			},
			wantStdoutContains: []string{"***.feature"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			r := &Runner{
				opts: Options{CaptureRaw: true},
				capture: &rawCapture{
					stdoutPath:   filepath.Join(dir, captureStdoutFileName),
					stderrPath:   filepath.Join(dir, captureStderrFileName),
					confidential: append([]string{}, tt.confidential...),
				},
			}

			tt.write(r)
			r.closeCapture()

			stdoutB, err := os.ReadFile(filepath.Join(dir, captureStdoutFileName))
			if err != nil {
				t.Fatalf("read stdout capture: %v", err)
			}
			stdout := string(stdoutB)
			for _, want := range tt.wantStdoutContains {
				if !strings.Contains(stdout, want) {
					t.Fatalf("stdout missing %q in %q", want, stdout)
				}
			}

			if tt.wantStderrExact != "" {
				stderrB, err := os.ReadFile(filepath.Join(dir, captureStderrFileName))
				if err != nil {
					t.Fatalf("read stderr capture: %v", err)
				}
				if got := string(stderrB); got != tt.wantStderrExact {
					t.Fatalf("unexpected stderr capture: %q", got)
				}
			}
		})
	}
}

func TestFallbackWriteToClosedFileReportsToStderr(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}

	r := &Runner{}
	errOut, _ := captureStderr(t, func() int {
		r.fallbackWrite(f, "payload")
		return 0
	})
	if !strings.Contains(errOut, "ccp: failed to write output") {
		t.Fatalf("expected fallback write error marker, got %q", errOut)
	}
}

func TestProcessLineRawModeBypassesEngine(t *testing.T) {
	r := &Runner{}
	r.rawMode = true

	stdout, _ := captureStdout(t, func() int {
		got := r.processLine("stdout", "tool", "", rawLineFixture, os.Stdout)
		if got != len(rawLineFixture) {
			t.Fatalf("processLine bytes = %d, want %d", got, len(rawLineFixture))
		}
		return 0
	})
	if stdout != rawLineFixture {
		t.Fatalf("unexpected raw-mode output: %q", stdout)
	}
}

func TestWriteOutputDebugMetadataBehavior(t *testing.T) {
	out := engine.Output{
		Output: "payload\n",
		Audit: engine.AuditRecord{
			Sequence:   7,
			DerivedKey: "k",
			Action:     engine.ActionFlush,
		},
	}
	tests := []struct {
		name     string
		debug    bool
		contains []string
		excludes []string
	}{
		{
			name:     "debug enabled includes metadata",
			debug:    true,
			contains: []string{"[SEQ:7][KEY:k][ACT:flush]", "payload"},
		},
		{
			name:     "debug disabled omits metadata",
			debug:    false,
			contains: []string{"payload"},
			excludes: []string{"[SEQ:", "[KEY:", "[ACT:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Runner{opts: Options{DebugFilter: tt.debug}}
			errOut, _ := captureStderr(t, func() int {
				r.writeOutput(os.Stderr, out)
				return 0
			})
			for _, expect := range tt.contains {
				if !strings.Contains(errOut, expect) {
					t.Fatalf("missing expected output fragment %q in %q", expect, errOut)
				}
			}
			for _, deny := range tt.excludes {
				if strings.Contains(errOut, deny) {
					t.Fatalf("unexpected output fragment %q in %q", deny, errOut)
				}
			}
		})
	}
}

func TestWriteOutputDebugMetadataRedactsConfidential(t *testing.T) {
	r := &Runner{opts: Options{DebugFilter: true, Confidential: []string{"secret"}}}
	out := engine.Output{
		Output: "secret-payload\n",
		Audit: engine.AuditRecord{
			Sequence:   7,
			DerivedKey: "secret-key",
			Action:     engine.ActionFlush,
		},
	}
	errOut, _ := captureStderr(t, func() int {
		_ = r.writeOutput(os.Stderr, out)
		return 0
	})
	if strings.Contains(errOut, "secret") {
		t.Fatalf("expected debug output to redact confidential content, got %q", errOut)
	}
	if !strings.Contains(errOut, "***-payload") || !strings.Contains(errOut, "***-key") {
		t.Fatalf("expected redacted debug output, got %q", errOut)
	}
}

func TestTickLoopFlushesBufferedOutput(t *testing.T) {
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(tickFlushFilter{runnerTestFilterBase: runnerTestFilterBase{tool: "ticktool"}}); err != nil {
		t.Fatalf("register filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	eng.SetCommandID("tick-cmd")
	_ = eng.Process(string(engine.StdoutStream), "ticktool", engine.Input{Line: "buffered\n"})
	r := &Runner{eng: eng}

	stdout := runTickLoopOnceAndCaptureStdout(t, r, "ticktool")
	if !strings.Contains(stdout, "tick-flush") {
		t.Fatalf("expected tick flush output, got %q", stdout)
	}
}

func TestRunnerForwardsDispatchKeyOnLineEOFAndExit(t *testing.T) {
	const dispatchKey = "dispatch-probe"
	f := &dispatchCaptureFilter{
		runnerTestFilterBase: runnerTestFilterBase{tool: shellToolForTests()},
		dispatchKey:          dispatchKey,
	}
	eng := engine.NewEngine(engine.Config{Filters: []engine.ToolFilter{f}})
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(f); err != nil {
		t.Fatalf("register dispatch filter: %v", err)
	}
	r := New(Options{}, eng, reg)

	_, code := captureStdout(t, func() int {
		if isWindows() {
			return r.Run([]string{"cmd.exe", "/C", "echo dispatch-line"})
		}
		return r.Run([]string{"sh", "-c", "printf 'dispatch-line\\n'"})
	})
	if code != 0 {
		t.Fatalf("expected success, got %d", code)
	}

	lineSeen, eofSeen, exitSeen, _, lineVal, eofVal, exitVal, _ := f.snapshot()
	if !lineSeen || !eofSeen || !exitSeen {
		t.Fatalf("expected line/eof/exit events, saw line=%t eof=%t exit=%t", lineSeen, eofSeen, exitSeen)
	}
	if lineVal != dispatchKey || eofVal != dispatchKey || exitVal != dispatchKey {
		t.Fatalf("expected dispatch key %q on line/eof/exit, got line=%q eof=%q exit=%q", dispatchKey, lineVal, eofVal, exitVal)
	}
}

func TestTickLoopPreservesDispatchKeyFromContext(t *testing.T) {
	const dispatchKey = "tick-dispatch-probe"
	f := &dispatchCaptureFilter{
		runnerTestFilterBase: runnerTestFilterBase{tool: "ticktool"},
		dispatchKey:          dispatchKey,
	}
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(f); err != nil {
		t.Fatalf("register filter: %v", err)
	}
	eng := engine.NewEngine(engine.Config{Registry: reg, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	eng.SetCommandID("tick-dispatch")
	_ = eng.Process(string(engine.StdoutStream), "ticktool", engine.Input{Line: "buffered\n", Dispatch: dispatchKey})

	r := &Runner{eng: eng}
	_ = runTickLoopOnceAndCaptureStdout(t, r, "ticktool")

	_, _, _, tickSeen, _, _, _, tickVal := f.snapshot()
	if !tickSeen {
		t.Fatal("expected tick event to be observed")
	}
	if tickVal != dispatchKey {
		t.Fatalf("expected tick dispatch key %q, got %q", dispatchKey, tickVal)
	}
}

func runTickLoopOnceAndCaptureStdout(t *testing.T, r *Runner, tool string) string {
	t.Helper()
	done := make(chan struct{})
	stdout, _ := captureStdout(t, func() int {
		go r.tickLoop(done, tool)
		time.Sleep(staleTickInterval + 100*time.Millisecond)
		close(done)
		time.Sleep(20 * time.Millisecond)
		return 0
	})
	return stdout
}

func TestCountLinesHandlesNoNewline(t *testing.T) {
	if got := countLines("single-line"); got != 1 {
		t.Fatalf("countLines without newline = %d, want 1", got)
	}
}
