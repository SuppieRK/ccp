package runner

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/metrics"
)

// Options configures command execution and filtering behavior.
type Options struct {
	Raw           bool
	CaptureRaw    bool
	CaptureRawDir string
	Confidential  []string
	DebugFilter   bool
	MetricsPath   string
}

// Runner executes prepared commands and routes stream output through the engine.
type Runner struct {
	opts     Options
	eng      *engine.Engine
	registry *engine.ToolFilterRegistry
	mu       sync.Mutex
	rawMode  bool
	capture  *rawCapture
}

type rawCapture struct {
	stdoutPath   string
	stderrPath   string
	stdoutFile   *os.File
	stderrFile   *os.File
	confidential []string
	seq          atomic.Int32
}

type sequencedCaptureWriter struct {
	file         *os.File
	seq          *atomic.Int32
	confidential []string
	buf          []byte
}

type streamStats struct {
	rawBytes  int
	keptBytes int
}

type runMetricsMeta struct {
	command         string
	tool            string
	engineDispatch  string
	metricsDispatch string
	code            int
	durationMS      int64
}

const (
	maxLineBytes      = 32 * 1024 * 1024
	streamReadBufSize = 64 * 1024
	staleTickInterval = 250 * time.Millisecond
)

// New creates a runner with optional semantic engine and shared registry.
func New(opts Options, eng *engine.Engine, registry *engine.ToolFilterRegistry) *Runner {
	initPlannerCapabilities()
	return &Runner{opts: opts, eng: eng, registry: registry}
}

// Registry returns the registry used for planning/execution in this runner.
func (r *Runner) Registry() *engine.ToolFilterRegistry {
	return r.registry
}

// Run executes one command line according to runner options and returns exit code.
func (r *Runner) Run(args []string) int {
	if r.opts.Raw {
		return r.runRaw(args)
	}

	plan, err := BuildExecPlan(args, r.registry)
	if err != nil {
		return writeStderrAndCode(1, err)
	}
	startedAt := time.Now().UTC()
	tool := plan.Tool
	engineDispatch := plan.DispatchKey
	metricsDispatch := withStdinDispatch(plan.DispatchKey)
	if plan.Name == "" {
		return writeStderrMsgAndCode(2, "no command provided")
	}
	if r.opts.CaptureRaw {
		capture, err := newRawCapture(r.opts.CaptureRawDir, r.opts.Confidential)
		if err != nil {
			return writeStderrAndCode(1, err)
		}
		r.capture = capture
		defer r.closeCapture()
	}

	cmd, stdout, stderr, code, err := startPlannedCommand(plan)
	if err != nil {
		return writeStderrAndCode(code, err)
	}
	if r.eng != nil {
		r.eng.SetCommandID(plan.RawInput)
	}

	done := make(chan struct{})
	if !r.opts.Raw && r.eng != nil {
		go r.tickLoop(done, tool)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	stdoutStats := &streamStats{}
	stderrStats := &streamStats{}
	go func() {
		defer wg.Done()
		r.copyStream("stdout", tool, engineDispatch, stdout, os.Stdout, stdoutStats)
	}()
	go func() {
		defer wg.Done()
		r.copyStream("stderr", tool, engineDispatch, stderr, os.Stderr, stderrStats)
	}()
	wg.Wait()
	close(done)

	exitCode, err := waitExitCode(cmd)
	if err != nil {
		return writeStderrAndCode(1, err)
	}
	duration := time.Since(startedAt).Milliseconds()
	r.emitExitAndMetrics(
		runMetricsMeta{
			command:         plan.RawInput,
			tool:            tool,
			engineDispatch:  engineDispatch,
			metricsDispatch: metricsDispatch,
			code:            exitCode,
			durationMS:      duration,
		},
		stdoutStats,
		stderrStats,
	)
	return exitCode
}

func directExecPlan(args []string) engine.ExecPlan {
	if len(args) == 0 {
		return engine.ExecPlan{}
	}
	name := args[0]
	tool := filepath.Base(name)
	return engine.ExecPlan{
		Tool:     tool,
		Name:     name,
		Args:     args[1:],
		RawInput: strings.Join(args, " "),
	}
}

func (r *Runner) runRaw(args []string) int {
	plan := directExecPlan(args)
	if plan.Name == "" {
		return writeStderrMsgAndCode(2, "no command provided")
	}
	var captureStdout, captureStderr *os.File

	if r.opts.CaptureRaw {
		capture, err := newRawCapture(r.opts.CaptureRawDir, r.opts.Confidential)
		if err != nil {
			return writeStderrAndCode(1, err)
		}
		r.capture = capture
		stdoutPath := capture.stdoutPath
		stderrPath := capture.stderrPath
		defer func() {
			r.closeCapture()
			removeEmptyCaptureFiles(stdoutPath, stderrPath)
		}()

		stdoutFile, err := os.OpenFile(capture.stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return writeStderrAndCode(1, err)
		}
		stderrFile, err := os.OpenFile(capture.stderrPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			_ = stdoutFile.Close()
			return writeStderrAndCode(1, err)
		}
		r.capture.stdoutFile = stdoutFile
		r.capture.stderrFile = stderrFile
		captureStdout = stdoutFile
		captureStderr = stderrFile
	}

	cmd, stdout, stderr, code, err := startDirectCommand(plan.Name, plan.Args)
	if err != nil {
		return writeStderrAndCode(code, err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		r.copyRawStream(stdout, os.Stdout, captureStdout)
	}()
	go func() {
		defer wg.Done()
		r.copyRawStream(stderr, os.Stderr, captureStderr)
	}()
	wg.Wait()

	exitCode, err := waitExitCode(cmd)
	if err != nil {
		return writeStderrAndCode(1, err)
	}
	return exitCode
}

func removeEmptyCaptureFiles(paths ...string) {
	for _, path := range paths {
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.Size() == 0 {
			_ = os.Remove(path)
		}
	}
}

func (w *sequencedCaptureWriter) Write(p []byte) (int, error) {
	if len(p) == 0 || w.file == nil || w.seq == nil {
		return len(p), nil
	}
	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := w.buf[:idx+1]
		if err := w.writeSequencedLine(line); err != nil {
			return len(p), err
		}
		w.buf = w.buf[idx+1:]
	}
	return len(p), nil
}

func (w *sequencedCaptureWriter) Flush() error {
	if len(w.buf) == 0 {
		return nil
	}
	if err := w.writeSequencedLine(w.buf); err != nil {
		return err
	}
	w.buf = nil
	return nil
}

func (w *sequencedCaptureWriter) writeSequencedLine(line []byte) error {
	seq := w.seq.Add(1) - 1
	if err := writeSequencePrefix(w.file, seq); err != nil {
		return err
	}
	_, err := io.WriteString(w.file, redactConfidential(string(line), w.confidential))
	return err
}

func commandWithPipes(name string, args []string) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		return nil, nil, nil, err
	}
	return cmd, stdout, stderr, nil
}

func withStdinDispatch(dispatch string) string {
	mode := detectStdinMode(os.Stdin)
	tag := "stdin=" + mode
	if dispatch == "" {
		return tag
	}
	parts := strings.Split(dispatch, "|")
	filtered := make([]string, 0, len(parts)+1)
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "stdin=") {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	filtered = append(filtered, tag)
	return strings.Join(filtered, "|")
}

func detectStdinMode(stdin *os.File) string {
	if stdin == nil {
		return "none"
	}
	info, err := stdin.Stat()
	if err != nil {
		return "none"
	}
	mode := info.Mode()
	if mode&os.ModeNamedPipe != 0 {
		return "pipe"
	}
	if mode&os.ModeCharDevice != 0 {
		return "tty"
	}
	if mode.IsRegular() {
		// File redirection is still stdin-fed input for command execution semantics.
		return "pipe"
	}
	return "none"
}

func closePipes(stdout, stderr io.ReadCloser) {
	if stdout != nil {
		_ = stdout.Close()
	}
	if stderr != nil {
		_ = stderr.Close()
	}
}

func startDirectCommand(name string, args []string) (*exec.Cmd, io.ReadCloser, io.ReadCloser, int, error) {
	cmd, stdout, stderr, err := commandWithPipes(name, args)
	if err != nil {
		return nil, nil, nil, 1, err
	}
	if err := cmd.Start(); err != nil {
		closePipes(stdout, stderr)
		return nil, nil, nil, 127, err
	}
	return cmd, stdout, stderr, 0, nil
}

func startPlannedCommand(plan engine.ExecPlan) (*exec.Cmd, io.ReadCloser, io.ReadCloser, int, error) {
	cmd, stdout, stderr, err := commandWithPipes(plan.Name, plan.Args)
	if err != nil {
		return nil, nil, nil, 1, err
	}
	if err := cmd.Start(); err == nil {
		return cmd, stdout, stderr, 0, nil
	} else if plan.FallbackName == "" {
		closePipes(stdout, stderr)
		return nil, nil, nil, 127, err
	}
	closePipes(stdout, stderr)

	cmd, stdout, stderr, err = commandWithPipes(plan.FallbackName, plan.FallbackArgs)
	if err != nil {
		return nil, nil, nil, 1, err
	}
	if err := cmd.Start(); err != nil {
		closePipes(stdout, stderr)
		return nil, nil, nil, 127, err
	}
	return cmd, stdout, stderr, 0, nil
}

func waitExitCode(cmd *exec.Cmd) (int, error) {
	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}

func (r *Runner) emitExitAndMetrics(meta runMetricsMeta, stdoutStats *streamStats, stderrStats *streamStats) {
	stdoutExitBytes, stderrExitBytes := r.emitExitEvent(meta.tool, meta.engineDispatch, meta.code)
	stdoutStats.keptBytes += stdoutExitBytes
	stderrStats.keptBytes += stderrExitBytes
	if !r.opts.Raw {
		_ = metrics.Append(r.opts.MetricsPath, metrics.RunMetric{
			Timestamp:   time.Now().UTC(),
			Command:     meta.command,
			Tool:        meta.tool,
			Dispatch:    meta.metricsDispatch,
			RawBytes:    stdoutStats.rawBytes + stderrStats.rawBytes,
			KeptBytes:   stdoutStats.keptBytes + stderrStats.keptBytes,
			ExitCode:    meta.code,
			DurationMS:  meta.durationMS,
			Passthrough: meta.tool == "",
		})
	}
}

func (r *Runner) copyRawStream(src io.Reader, dst *os.File, captureFile *os.File) {
	target := io.Writer(dst)
	var capWriter *sequencedCaptureWriter
	if captureFile != nil && r.capture != nil {
		capWriter = &sequencedCaptureWriter{
			file:         captureFile,
			seq:          &r.capture.seq,
			confidential: r.capture.confidential,
		}
		target = io.MultiWriter(dst, capWriter)
	}
	_, _ = io.Copy(target, src)
	if capWriter != nil {
		_ = capWriter.Flush()
	}
}

func (r *Runner) emitExitEvent(tool string, dispatch string, code int) (stdoutBytes int, stderrBytes int) {
	if r.opts.Raw || r.eng == nil {
		return 0, 0
	}
	out := r.eng.Process(string(engine.StdoutStream), tool, engine.Input{Dispatch: dispatch, Exit: true, Code: code})
	if out.Ready && out.Output != "" {
		target := os.Stdout
		size := len(out.Output)
		if out.Stream == engine.StderrStream {
			target = os.Stderr
			stderrBytes += size
		} else {
			stdoutBytes += size
		}
		r.writeOutput(target, out)
	}
	return stdoutBytes, stderrBytes
}

func (r *Runner) copyStream(name string, tool string, dispatch string, src io.Reader, dst *os.File, stats *streamStats) {
	reader := bufio.NewReaderSize(src, streamReadBufSize)
	for {
		line, overflow, err := readLineBounded(reader, maxLineBytes)
		if len(line) > 0 {
			stats.rawBytes += len(line)
			stats.keptBytes += r.processLine(name, tool, dispatch, line, dst)
		}
		if overflow {
			marker := "[..., context limit reached; oversized line truncated ...]\n"
			stats.keptBytes += r.processLine(name, tool, dispatch, marker, dst)
		}
		if err != nil {
			if err == io.EOF {
				stats.keptBytes += r.processLine(name, tool, dispatch, "", dst)
				return
			}
			r.fallbackWrite(dst, line)
			return
		}
	}
}

func readLineBounded(r *bufio.Reader, max int) (string, bool, error) {
	var out []byte
	remaining := max
	overflow := false
	for {
		chunk, err := r.ReadSlice('\n')
		if line, lineOverflow, ok := boundedSingleChunkLine(chunk, out, max, err); ok {
			return line, lineOverflow, err
		}
		out, remaining, overflow = appendBoundedChunk(out, chunk, remaining, overflow)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return string(out), overflow, err
	}
}

func boundedSingleChunkLine(chunk, out []byte, max int, err error) (string, bool, bool) {
	if len(chunk) == 0 || out != nil || errors.Is(err, bufio.ErrBufferFull) {
		return "", false, false
	}
	if len(chunk) <= max {
		return string(chunk), false, true
	}
	return string(chunk[:max]), true, true
}

func appendBoundedChunk(out, chunk []byte, remaining int, overflow bool) ([]byte, int, bool) {
	if len(chunk) == 0 {
		return out, remaining, overflow
	}
	if remaining <= 0 {
		return out, remaining, true
	}
	if len(chunk) <= remaining {
		return append(out, chunk...), remaining - len(chunk), overflow
	}
	return append(out, chunk[:remaining]...), 0, true
}

func (r *Runner) processLine(stream, tool, dispatch, line string, dst *os.File) int {
	isEOF := line == ""
	defer func() {
		if recover() != nil {
			r.mu.Lock()
			r.rawMode = true
			r.mu.Unlock()
			if line != "" {
				r.fallbackWrite(dst, line)
			}
		}
	}()

	if r.isRawMode() || r.opts.Raw {
		return r.processLineRawPassthrough(stream, line, dst)
	}

	// Normalize terminal formatting noise once at runtime ingress.
	if line != "" {
		line = stripANSI(line)
	}
	if r.eng == nil {
		return r.writeLineIfNonEmpty(dst, line)
	}
	out := r.eng.Process(stream, tool, engine.Input{Line: line, Dispatch: dispatch, EOF: isEOF})
	if out.Ready && out.Output != "" {
		target := dst
		switch out.Stream {
		case engine.StderrStream:
			target = os.Stderr
		case engine.StdoutStream:
			target = os.Stdout
		}
		r.writeOutput(target, out)
		return len(out.Output)
	}
	return 0
}

func (r *Runner) processLineRawPassthrough(stream, line string, dst *os.File) int {
	if line == "" {
		return 0
	}
	r.captureRawLine(stream, line)
	r.fallbackWrite(dst, line)
	return len(line)
}

func (r *Runner) writeLineIfNonEmpty(dst *os.File, line string) int {
	if line == "" {
		return 0
	}
	r.fallbackWrite(dst, line)
	return len(line)
}

func (r *Runner) isRawMode() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rawMode
}

func (r *Runner) tickLoop(done <-chan struct{}, tool string) {
	ticker := time.NewTicker(staleTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			outs := r.eng.ProcessTick(tool)
			for _, out := range outs {
				if !out.Ready || out.Output == "" {
					continue
				}
				if out.Stream == engine.StderrStream {
					r.writeOutput(os.Stderr, out)
				} else {
					r.writeOutput(os.Stdout, out)
				}
			}
		}
	}
}

func (r *Runner) writeOutput(dst *os.File, out engine.Output) {
	if r.opts.DebugFilter {
		meta := fmt.Sprintf("[SEQ:%d][KEY:%s][ACT:%s] ", out.Audit.Sequence, out.Audit.DerivedKey, out.Audit.Action)
		r.fallbackWrite(os.Stderr, meta)
	}
	r.fallbackWrite(dst, out.Output)
}

func newRawCapture(dir string, confidential []string) (*rawCapture, error) {
	now := time.Now().UTC()
	stamp := now.Format("20060102-150405.000000000")
	baseDir := "."
	if strings.TrimSpace(dir) != "" {
		baseDir = dir
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, err
	}
	stdoutPath := filepath.Join(baseDir, "ccp-capture-"+stamp+"-input-stdout.txt")
	stderrPath := filepath.Join(baseDir, "ccp-capture-"+stamp+"-input-stderr.txt")
	return &rawCapture{
		stdoutPath:   stdoutPath,
		stderrPath:   stderrPath,
		confidential: append([]string(nil), confidential...),
	}, nil
}

func (r *Runner) closeCapture() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.capture == nil {
		return
	}
	if r.capture.stdoutFile != nil {
		_ = r.capture.stdoutFile.Close()
	}
	if r.capture.stderrFile != nil {
		_ = r.capture.stderrFile.Close()
	}
	r.capture = nil
}

func (r *Runner) captureRawLine(stream, line string) {
	if !r.opts.CaptureRaw || line == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.capture == nil {
		return
	}
	f, ok := r.ensureCaptureFileLocked(stream)
	if !ok || f == nil {
		return
	}
	seq := r.capture.seq.Add(1) - 1
	_ = writeSequencePrefix(f, seq)
	_, _ = io.WriteString(f, redactConfidential(line, r.capture.confidential))
}

func redactConfidential(input string, confidential []string) string {
	if input == "" || len(confidential) == 0 {
		return input
	}
	out := input
	for _, token := range confidential {
		if token == "" {
			continue
		}
		out = strings.ReplaceAll(out, token, "***")
	}
	return out
}

func (r *Runner) ensureCaptureFileLocked(stream string) (*os.File, bool) {
	var (
		target **os.File
		path   string
	)
	switch stream {
	case "stdout":
		target = &r.capture.stdoutFile
		path = r.capture.stdoutPath
	case "stderr":
		target = &r.capture.stderrFile
		path = r.capture.stderrPath
	default:
		return nil, false
	}

	if *target == nil {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return nil, false
		}
		*target = f
	}
	return *target, true
}

func writeSequencePrefix(f *os.File, seq int32) error {
	var nbuf [16]byte
	digits := strconv.AppendInt(nbuf[:0], int64(seq), 10)
	if len(digits) >= 5 {
		if _, err := f.Write(digits); err != nil {
			return err
		}
		_, err := f.Write([]byte{'|'})
		return err
	}

	var out [6]byte
	pad := 5 - len(digits)
	for i := 0; i < pad; i++ {
		out[i] = '0'
	}
	copy(out[pad:], digits)
	out[5] = '|'
	_, err := f.Write(out[:])
	return err
}

func (r *Runner) fallbackWrite(dst *os.File, line string) {
	if _, err := io.WriteString(dst, line); err != nil {
		if dst != os.Stderr {
			if _, stderrErr := io.WriteString(os.Stderr, "ccp: failed to write output\n"); stderrErr != nil {
				return
			}
		}
	}
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	c := strings.Count(s, "\n")
	if c == 0 {
		return 1
	}
	return c
}

func writeStderrAndCode(code int, err error) int {
	if err == nil {
		return code
	}
	return writeStderrMsgAndCode(code, err.Error())
}

func writeStderrMsgAndCode(code int, msg string) int {
	if _, err := fmt.Fprintln(os.Stderr, msg); err != nil {
		return 1
	}
	return code
}
