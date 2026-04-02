package core

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go-command-compression-proxy/internal/audit"
	"go-command-compression-proxy/internal/cli"
	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/engine"
	corefilters "go-command-compression-proxy/internal/filters"
	filteryaml "go-command-compression-proxy/internal/filters/yaml"
	"go-command-compression-proxy/internal/metrics"
	"go-command-compression-proxy/internal/workspaces"
)

type Options struct {
	Raw          bool
	Confidential []string
	MetricsPath  string
}

type Runner struct {
	sources     []corefilters.FilterSource
	metricsPath string
	workingDir  string
	opts        Options
}

type entrySink func([]engine.BufferEntry) (int, error)

type redactingWriter struct {
	writer       io.Writer
	confidential []string
	buf          []byte
}

func NewRunnerWithOptions(opts Options) *Runner {
	metricsPath := opts.MetricsPath
	if strings.TrimSpace(metricsPath) == "" {
		metricsPath = defaultMetricsPath()
	}
	return &Runner{
		sources:     filteryaml.DefaultSources(),
		metricsPath: metricsPath,
		workingDir:  currentWorkingDir(),
		opts:        opts,
	}
}

func (r *Runner) Run(args []string) (int, error) {
	var parent context.Context
	return r.run(parent, args)
}

func (r *Runner) RunContext(parent context.Context, args []string) (int, error) {
	if parent == nil {
		return r.Run(args)
	}
	return r.run(parent, args)
}

func (r *Runner) run(parent context.Context, args []string) (int, error) {
	ctx, stop := runnerContext(parent)
	defer stop()

	command, err := ParseCommandArgs(args)
	if err != nil {
		return 2, err
	}
	if r.opts.Raw {
		return r.runRaw(ctx, command, args)
	}
	startedAt := time.Now().UTC()
	shape := cli.DescribeExecutionShape(args)
	auditCommand := r.auditCommand(command.RawInput)
	if err := audit.Append("execution_start", map[string]any{
		"command":       auditCommand,
		"tool":          command.Tool,
		"raw":           false,
		"uses_shell":    shape.UsesShell,
		"has_pipeline":  shape.HasPipeline,
		"has_chain":     shape.HasChain,
		"has_find_exec": shape.HasFindExec,
		"has_xargs":     shape.HasXargs,
		"nested_ccp":    shape.NestedCCP,
	}); err != nil {
		return 1, err
	}

	registry, err := r.loadExecutionRegistry(auditCommand, command.Tool)
	if err != nil {
		return 1, err
	}
	resolved := registry.Resolve(command)
	command, err = resolved.PrepareCommand(command)
	if err != nil {
		return 1, err
	}
	command.Dispatch = resolved.Dispatch(command)
	state := engine.NewEngine(registry).Start(command)
	cmd, stdout, stderr, err := CommandWithPipesContext(ctx, command.Args[0], command.Args[1:])
	if err != nil {
		return 1, err
	}
	if err := cmd.Start(); err != nil {
		closePipes(stdout, stderr)
		return 127, err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	stdoutStats := &streamStats{}
	stderrStats := &streamStats{}
	var stdoutWriteErr error
	var stderrWriteErr error
	go func() {
		defer wg.Done()
		stdoutWriteErr = r.drainStream(stdout, state.Stdout, stdoutStats, r.writeEntries)
	}()
	go func() {
		defer wg.Done()
		stderrWriteErr = r.drainStream(stderr, state.Stderr, stderrStats, r.writeEntries)
	}()
	wg.Wait()

	exitCode, err := waitExitCode(cmd)
	exitWritten, exitWriteErr := r.writeEntries(state.Exit(exitCode))
	outputErr := errors.Join(stdoutWriteErr, stderrWriteErr, exitWriteErr)
	if code, runErr := filteredRunResult(ctx, err, outputErr, exitCode); runErr != nil {
		return code, runErr
	}
	keptBytes := stdoutStats.keptBytes + stderrStats.keptBytes + exitWritten
	rawBytes := stdoutStats.rawBytes + stderrStats.rawBytes
	r.appendMetrics(command, isPassthroughFilter(resolved, command), exitCode, time.Since(startedAt).Milliseconds(), rawBytes, keptBytes)
	auditErr := audit.Append("execution_finish", map[string]any{
		"command":     auditCommand,
		"tool":        command.Tool,
		"dispatch":    command.Dispatch,
		"raw":         false,
		"exit_code":   exitCode,
		"duration_ms": time.Since(startedAt).Milliseconds(),
		"raw_bytes":   rawBytes,
		"kept_bytes":  keptBytes,
	})
	if auditErr != nil {
		return auditFailureResult(exitCode, auditErr)
	}
	return exitCode, nil
}

func (r *Runner) loadExecutionRegistry(auditCommand, tool string) (*engine.Registry, error) {
	registry, err := r.loadRegistry()
	if err == nil {
		return registry, nil
	}
	if auditErr := audit.Append("execution_registry_error", map[string]any{
		"command": auditCommand,
		"tool":    tool,
		"error":   err.Error(),
	}); auditErr != nil {
		return nil, errors.Join(err, auditErr)
	}
	return engine.NewRegistry(), nil
}

func filteredRunResult(ctx context.Context, waitErr, outputErr error, exitCode int) (int, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return 1, errors.Join(ctxErr, outputErr)
	}
	if waitErr != nil {
		return 1, errors.Join(waitErr, outputErr)
	}
	if outputErr == nil {
		return exitCode, nil
	}
	if exitCode == 0 {
		return 1, outputErr
	}
	return exitCode, outputErr
}

func auditFailureResult(exitCode int, err error) (int, error) {
	if exitCode == 0 {
		return 1, err
	}
	return exitCode, err
}

func (r *Runner) runRaw(ctx context.Context, command contracts.Command, args []string) (int, error) {
	startedAt := time.Now().UTC()
	shape := cli.DescribeExecutionShape(args)
	auditCommand := r.auditCommand(command.RawInput)
	if err := audit.Append("execution_start", map[string]any{
		"command":       auditCommand,
		"tool":          command.Tool,
		"raw":           true,
		"uses_shell":    shape.UsesShell,
		"has_pipeline":  shape.HasPipeline,
		"has_chain":     shape.HasChain,
		"has_find_exec": shape.HasFindExec,
		"has_xargs":     shape.HasXargs,
		"nested_ccp":    shape.NestedCCP,
	}); err != nil {
		return 1, err
	}

	cmd, stdout, stderr, err := CommandWithPipesContext(ctx, command.Args[0], command.Args[1:])
	if err != nil {
		return 1, err
	}
	if err := cmd.Start(); err != nil {
		closePipes(stdout, stderr)
		return 127, err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	var stdoutWriteErr error
	var stderrWriteErr error
	go func() {
		defer wg.Done()
		stdoutWriteErr = r.copyRawStream(stdout, os.Stdout)
	}()
	go func() {
		defer wg.Done()
		stderrWriteErr = r.copyRawStream(stderr, os.Stderr)
	}()
	wg.Wait()

	exitCode, err := waitExitCode(cmd)
	outputErr := errors.Join(stdoutWriteErr, stderrWriteErr)
	if ctx.Err() != nil {
		return 1, errors.Join(ctx.Err(), outputErr)
	}
	auditErr := audit.Append("execution_finish", map[string]any{
		"command":     auditCommand,
		"tool":        command.Tool,
		"raw":         true,
		"exit_code":   exitCode,
		"duration_ms": time.Since(startedAt).Milliseconds(),
	})
	if auditErr != nil {
		if exitCode == 0 {
			return 1, errors.Join(outputErr, auditErr)
		}
		return exitCode, errors.Join(outputErr, auditErr)
	}
	if outputErr != nil {
		if exitCode == 0 {
			return 1, outputErr
		}
		return exitCode, outputErr
	}
	return exitCode, err
}

func (r *Runner) loadRegistry() (*engine.Registry, error) {
	registry := engine.NewRegistry()
	filters, err := filteryaml.LoadRegistryFiltersFromSources(r.sources)
	if err != nil {
		return nil, err
	}
	registry.RegisterAll(filters)
	return registry, nil
}

func defaultMetricsPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(cwd, ".ccp", "gain.db")
}

type streamStats struct {
	rawBytes  int
	keptBytes int
}

func (r *Runner) drainStream(src io.Reader, consume func(string) []engine.BufferEntry, stats *streamStats, sink entrySink) error {
	if src == nil {
		return nil
	}
	reader := bufio.NewReader(src)
	var currentLine []byte
	pendingCR := false
	var sinkErr error
	for {
		b, err := reader.ReadByte()
		if err != nil {
			r.finishDrainedStream(currentLine, pendingCR, consume, stats, sink, &sinkErr)
			return errors.Join(sinkErr, wrapStreamReadError(err))
		}
		recordRawByte(stats)
		if r.handlePendingCRByte(b, consume, stats, sink, &pendingCR, &currentLine, &sinkErr) {
			continue
		}
		r.consumeStreamByte(b, consume, stats, sink, &pendingCR, &currentLine, &sinkErr)
	}
}

func recordRawByte(stats *streamStats) {
	if stats != nil {
		stats.rawBytes++
	}
}

func recordKeptBytes(stats *streamStats, written int) {
	if stats != nil {
		stats.keptBytes += written
	}
}

func (r *Runner) finishDrainedStream(
	currentLine []byte,
	pendingCR bool,
	consume func(string) []engine.BufferEntry,
	stats *streamStats,
	sink entrySink,
	sinkErr *error,
) {
	if pendingCR || len(currentLine) == 0 {
		return
	}
	written, err := sink(consume(string(currentLine)))
	r.recordSinkResult(stats, written, err, sinkErr)
}

func (r *Runner) handlePendingCRByte(
	b byte,
	consume func(string) []engine.BufferEntry,
	stats *streamStats,
	sink entrySink,
	pendingCR *bool,
	currentLine *[]byte,
	sinkErr *error,
) bool {
	if !*pendingCR {
		return false
	}
	if b == '\n' {
		r.emitConsumedLine(currentLine, true, consume, stats, sink, sinkErr)
		*pendingCR = false
		return true
	}
	*currentLine = (*currentLine)[:0]
	*pendingCR = false
	return false
}

func (r *Runner) consumeStreamByte(
	b byte,
	consume func(string) []engine.BufferEntry,
	stats *streamStats,
	sink entrySink,
	pendingCR *bool,
	currentLine *[]byte,
	sinkErr *error,
) {
	switch b {
	case '\r':
		*pendingCR = true
	case '\n':
		r.emitConsumedLine(currentLine, true, consume, stats, sink, sinkErr)
	default:
		*currentLine = append(*currentLine, b)
	}
}

func (r *Runner) emitConsumedLine(
	currentLine *[]byte,
	includeNewline bool,
	consume func(string) []engine.BufferEntry,
	stats *streamStats,
	sink entrySink,
	sinkErr *error,
) {
	if includeNewline {
		*currentLine = append(*currentLine, '\n')
	}
	written, err := sink(consume(string(*currentLine)))
	r.recordSinkResult(stats, written, err, sinkErr)
	*currentLine = (*currentLine)[:0]
}

func (r *Runner) recordSinkResult(stats *streamStats, written int, err error, sinkErr *error) {
	recordKeptBytes(stats, written)
	if err != nil && sinkErr != nil && *sinkErr == nil {
		*sinkErr = err
	}
}

func (r *Runner) writeEntries(entries []engine.BufferEntry) (int, error) {
	written := 0
	for _, entry := range entries {
		var (
			lineWritten int
			err         error
		)
		switch entry.Stream {
		case "stderr":
			lineWritten, err = r.writeRedacted(os.Stderr, entry.Line)
		default:
			lineWritten, err = r.writeRedacted(os.Stdout, entry.Line)
		}
		written += lineWritten
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

func closePipes(stdout, stderr io.ReadCloser) {
	if stdout != nil {
		_ = stdout.Close()
	}
	if stderr != nil {
		_ = stderr.Close()
	}
}

func waitExitCode(cmd *exec.Cmd) (int, error) {
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}

type commandPassthroughReporter interface {
	ReportsPassthrough(command contracts.Command) bool
}

func isPassthroughFilter(filter any, command contracts.Command) bool {
	switch f := filter.(type) {
	case corefilters.Passthrough, *corefilters.Passthrough:
		return true
	case commandPassthroughReporter:
		return f.ReportsPassthrough(command)
	default:
		return false
	}
}

func (r *Runner) appendMetrics(command contracts.Command, passthrough bool, exitCode int, durationMS int64, rawBytes, keptBytes int) {
	if r.opts.Raw {
		return
	}
	if !shouldRecordMetrics(command) {
		return
	}
	if err := metrics.Append(r.metricsPath, metrics.RunMetric{
		Timestamp:   time.Now().UTC(),
		Command:     command.RawInput,
		Tool:        command.Tool,
		Dispatch:    command.Dispatch,
		RawBytes:    rawBytes,
		KeptBytes:   keptBytes,
		ExitCode:    exitCode,
		DurationMS:  durationMS,
		Passthrough: passthrough,
	}); err != nil {
		return
	}
	if strings.TrimSpace(r.workingDir) == "" {
		return
	}
	path, err := workspaces.DefaultPath()
	if err != nil {
		return
	}
	_ = workspaces.UpsertPath(path, r.workingDir, r.metricsPath)
}

func shouldRecordMetrics(command contracts.Command) bool {
	return !cli.ShouldSkipMetrics(command.Tool, command.Args)
}

func currentWorkingDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

func (r *Runner) writeRedacted(dst *os.File, line string) (int, error) {
	redacted := redactConfidential(line, r.opts.Confidential)
	if _, err := io.WriteString(dst, redacted); err != nil {
		return 0, fmt.Errorf("write %s: %w", outputName(dst), err)
	}
	return len(redacted), nil
}

func (r *Runner) auditCommand(raw string) string {
	return redactConfidential(raw, r.opts.Confidential)
}

func (r *Runner) copyRawStream(src io.Reader, dst *os.File) error {
	recorder := &errorRecordingWriter{writer: dst, name: outputName(dst)}
	target := io.Writer(recorder)
	var outWriter *redactingWriter
	if len(r.opts.Confidential) > 0 {
		outWriter = &redactingWriter{
			writer:       recorder,
			confidential: r.opts.Confidential,
		}
		target = outWriter
	}
	_, copyErr := io.Copy(target, src)
	var flushErr error
	if outWriter != nil {
		flushErr = outWriter.Flush()
	}
	return errors.Join(recorder.err, wrapStreamReadError(copyErr), flushErr)
}

func wrapStreamReadError(err error) error {
	if err == nil || errors.Is(err, io.EOF) {
		return nil
	}
	return fmt.Errorf("read stream: %w", err)
}

type errorRecordingWriter struct {
	writer io.Writer
	name   string
	err    error
}

func (w *errorRecordingWriter) Write(p []byte) (int, error) {
	if len(p) == 0 || w.writer == nil {
		return len(p), nil
	}
	if w.err != nil {
		return len(p), nil
	}
	_, err := w.writer.Write(p)
	if err != nil {
		w.err = fmt.Errorf("write %s: %w", w.name, err)
		return len(p), nil
	}
	return len(p), nil
}

func outputName(dst *os.File) string {
	if dst == os.Stderr {
		return "stderr"
	}
	return "stdout"
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

func (w *redactingWriter) Write(p []byte) (int, error) {
	if len(p) == 0 || w.writer == nil {
		return len(p), nil
	}
	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := w.buf[:idx+1]
		if err := w.writeRedactedLine(line); err != nil {
			return len(p), err
		}
		w.buf = w.buf[idx+1:]
	}
	return len(p), nil
}

func (w *redactingWriter) Flush() error {
	if len(w.buf) == 0 || w.writer == nil {
		return nil
	}
	if err := w.writeRedactedLine(w.buf); err != nil {
		return err
	}
	w.buf = nil
	return nil
}

func (w *redactingWriter) writeRedactedLine(line []byte) error {
	_, err := io.WriteString(w.writer, redactConfidential(string(line), w.confidential))
	return err
}
