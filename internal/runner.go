package core

import (
	"bufio"
	"bytes"
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
	"go-command-compression-proxy/internal/replay"
	"go-command-compression-proxy/internal/version"
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

type ReplayResult struct {
	Output    string
	Decisions string
}

type entrySink func([]engine.BufferEntry) (int, error)

type redactingWriter struct {
	writer       io.Writer
	confidential []string
	buf          []byte
}

func NewRunner() *Runner {
	return NewRunnerWithOptions(Options{})
}

func NewRunnerWithOptions(opts Options) *Runner {
	metricsPath := opts.MetricsPath
	if strings.TrimSpace(metricsPath) == "" {
		metricsPath = defaultMetricsPath()
	}
	return &Runner{
		sources:     defaultFilterSources(),
		metricsPath: metricsPath,
		workingDir:  currentWorkingDir(),
		opts:        opts,
	}
}

func (r *Runner) Run(args []string) (int, error) {
	command, err := ParseCommandArgs(args)
	if err != nil {
		return 2, err
	}
	if r.opts.Raw {
		return r.runRaw(command, args)
	}
	startedAt := time.Now().UTC()
	shape := cli.DescribeExecutionShape(args)
	audit.MustAppend("execution_start", map[string]any{
		"command":       command.RawInput,
		"tool":          command.Tool,
		"raw":           false,
		"uses_shell":    shape.UsesShell,
		"has_pipeline":  shape.HasPipeline,
		"has_chain":     shape.HasChain,
		"has_find_exec": shape.HasFindExec,
		"has_xargs":     shape.HasXargs,
		"nested_ccp":    shape.NestedCCP,
	})

	registry, err := r.loadRegistry()
	if err != nil {
		audit.MustAppend("execution_registry_error", map[string]any{
			"command": command.RawInput,
			"tool":    command.Tool,
			"error":   err.Error(),
		})
		return 1, err
	}
	resolved := registry.Resolve(command)
	command, err = resolved.PrepareCommand(command)
	if err != nil {
		return 1, err
	}
	command.Dispatch = resolved.Dispatch(command)
	state := engine.NewEngine(registry).Start(command)
	cmd, stdout, stderr, err := commandWithPipes(command.Args[0], command.Args[1:])
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
	if err != nil {
		return 1, errors.Join(err, outputErr)
	}
	if outputErr != nil {
		if exitCode == 0 {
			return 1, outputErr
		}
		return exitCode, outputErr
	}
	keptBytes := stdoutStats.keptBytes + stderrStats.keptBytes + exitWritten
	rawBytes := stdoutStats.rawBytes + stderrStats.rawBytes
	r.appendMetrics(command, isPassthroughFilter(resolved), exitCode, time.Since(startedAt).Milliseconds(), rawBytes, keptBytes)
	audit.MustAppend("execution_finish", map[string]any{
		"command":     command.RawInput,
		"tool":        command.Tool,
		"dispatch":    command.Dispatch,
		"raw":         false,
		"exit_code":   exitCode,
		"duration_ms": time.Since(startedAt).Milliseconds(),
		"raw_bytes":   rawBytes,
		"kept_bytes":  keptBytes,
	})
	return exitCode, nil
}

func (r *Runner) runRaw(command contracts.Command, args []string) (int, error) {
	startedAt := time.Now().UTC()
	shape := cli.DescribeExecutionShape(args)
	audit.MustAppend("execution_start", map[string]any{
		"command":       command.RawInput,
		"tool":          command.Tool,
		"raw":           true,
		"uses_shell":    shape.UsesShell,
		"has_pipeline":  shape.HasPipeline,
		"has_chain":     shape.HasChain,
		"has_find_exec": shape.HasFindExec,
		"has_xargs":     shape.HasXargs,
		"nested_ccp":    shape.NestedCCP,
	})

	cmd, stdout, stderr, err := commandWithPipes(command.Args[0], command.Args[1:])
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
	audit.MustAppend("execution_finish", map[string]any{
		"command":     command.RawInput,
		"tool":        command.Tool,
		"raw":         true,
		"exit_code":   exitCode,
		"duration_ms": time.Since(startedAt).Milliseconds(),
	})
	if outputErr != nil {
		if exitCode == 0 {
			return 1, outputErr
		}
		return exitCode, outputErr
	}
	return exitCode, err
}

func (r *Runner) Verify(args []string, stdout, stderr io.Reader) (string, error) {
	events, err := replayEventsFromReaders(stdout, stderr)
	if err != nil {
		return "", err
	}
	result, err := r.Replay(args, events)
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

func (r *Runner) Replay(args []string, events []replay.Event) (ReplayResult, error) {
	return r.ReplayWithExitCode(args, events, 0)
}

func (r *Runner) ReplayWithExitCode(args []string, events []replay.Event, exitCode int) (ReplayResult, error) {
	command, err := ParseCommandArgs(args)
	if err != nil {
		return ReplayResult{}, err
	}
	audit.MustAppend("verify_start", map[string]any{
		"command": command.RawInput,
		"tool":    command.Tool,
	})

	registry, err := r.loadRegistry()
	if err != nil {
		audit.MustAppend("verify_registry_error", map[string]any{
			"command": command.RawInput,
			"tool":    command.Tool,
			"error":   err.Error(),
		})
		return ReplayResult{}, err
	}
	resolved := registry.Resolve(command)
	command, err = resolved.PrepareCommand(command)
	if err != nil {
		return ReplayResult{}, err
	}
	command.Dispatch = resolved.Dispatch(command)
	state := engine.NewEngine(registry).Start(command)
	collector := &replayCollector{}
	for _, event := range events {
		var (
			action  contracts.Action
			entries []engine.BufferEntry
		)
		switch event.Stream {
		case contracts.StreamStderr:
			action, entries = state.StderrAction(event.Line)
		default:
			action, entries = state.StdoutAction(event.Line)
		}
		collector.recordInput(event, action, entries)
	}
	exitAction, exitEntries := state.ExitAction(exitCode)
	collector.recordExit(exitAction, exitEntries)
	audit.MustAppend("verify_finish", map[string]any{
		"command":      command.RawInput,
		"tool":         command.Tool,
		"dispatch":     command.Dispatch,
		"output_bytes": len(collector.output.String()),
	})
	return ReplayResult{
		Output:    collector.output.String(),
		Decisions: collector.decisions.String(),
	}, nil
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

func defaultFilterSources() []corefilters.FilterSource {
	if version.Version == "dev" {
		return []corefilters.FilterSource{
			corefilters.RepositorySource(filteryaml.ProjectRootFromSource()),
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return []corefilters.FilterSource{
			corefilters.ProjectSource(cwd),
		}
	}
	return []corefilters.FilterSource{
		corefilters.ProjectSource(cwd),
		corefilters.HomeSource(home),
	}
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
			return sinkErr
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

func (r *Runner) copyStream(src io.Reader, consume func(string) []engine.BufferEntry, stats *streamStats) error {
	return r.drainStream(src, consume, stats, r.writeEntries)
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

type replayCollector struct {
	output    bytes.Buffer
	decisions bytes.Buffer
}

func (c *replayCollector) writeEntries(entries []engine.BufferEntry) int {
	written := 0
	for _, entry := range entries {
		written += len(entry.Line)
		_, _ = c.output.WriteString(entry.Line)
	}
	return written
}

func (c *replayCollector) recordInput(event replay.Event, action contracts.Action, emitted []engine.BufferEntry) {
	c.writeDecision(labelForInputAction(action), event.Line)
	c.writeEntries(emitted)
	if action.Kind == contracts.ActionReplace {
		c.writeSynthetic(emitted)
	}
}

func (c *replayCollector) recordExit(_ contracts.Action, emitted []engine.BufferEntry) {
	c.writeEntries(emitted)
}

func (c *replayCollector) writeSynthetic(entries []engine.BufferEntry) {
	for _, entry := range entries {
		c.writeDecision("<emit>", entry.Line)
	}
}

func (c *replayCollector) writeDecision(label, line string) {
	for _, part := range splitDecisionLines(line) {
		text := strings.TrimSuffix(part, "\n")
		_, _ = fmt.Fprintf(&c.decisions, "%-10s| %s\n", label, text)
	}
}

func labelForInputAction(action contracts.Action) string {
	switch action.Kind {
	case contracts.ActionIgnore:
		return "<skip>"
	case contracts.ActionReplace:
		return "<replace>"
	case contracts.ActionEmit, contracts.ActionKeep, "":
		return "<keep>"
	default:
		return "<keep>"
	}
}

func splitDecisionLines(line string) []string {
	if line == "" {
		return []string{""}
	}
	parts := strings.SplitAfter(strings.ReplaceAll(line, "\r\n", "\n"), "\n")
	if parts[len(parts)-1] == "" {
		return parts[:len(parts)-1]
	}
	return parts
}

func replayEventsFromReaders(stdout, stderr io.Reader) ([]replay.Event, error) {
	var events []replay.Event
	sequence := 0
	for _, current := range []struct {
		stream contracts.Stream
		reader io.Reader
	}{
		{stream: contracts.StreamStdout, reader: stdout},
		{stream: contracts.StreamStderr, reader: stderr},
	} {
		if current.reader == nil {
			continue
		}
		lines, err := readReplayLines(current.reader)
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			events = append(events, replay.Event{
				Sequence: sequence,
				Stream:   current.stream,
				Line:     line,
			})
			sequence++
		}
	}
	return events, nil
}

func readReplayLines(src io.Reader) ([]string, error) {
	reader := bufio.NewReader(src)
	lines := make([]string, 0, 32)
	var currentLine []byte
	pendingCR := false
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return finishReplayLines(lines, currentLine, pendingCR, err)
		}
		line, emitted, nextPendingCR := appendReplayLineByte(currentLine, b, pendingCR)
		if emitted {
			lines = append(lines, line)
			currentLine = currentLine[:0]
			pendingCR = false
			continue
		}
		if b != '\r' {
			currentLine = append(currentLine, b)
		}
		pendingCR = nextPendingCR
	}
}

func appendReplayLineByte(currentLine []byte, b byte, pendingCR bool) (string, bool, bool) {
	if pendingCR {
		if b == '\n' {
			return string(append(currentLine, '\n')), true, false
		}
		currentLine = currentLine[:0]
	}

	switch b {
	case '\r':
		return "", false, true
	case '\n':
		return string(append(currentLine, '\n')), true, false
	default:
		return "", false, false
	}
}

func finishReplayLines(lines []string, currentLine []byte, pendingCR bool, err error) ([]string, error) {
	if pendingCR {
		currentLine = currentLine[:0]
	}
	if len(currentLine) > 0 {
		lines = append(lines, string(currentLine))
	}
	if err == io.EOF {
		return lines, nil
	}
	return nil, err
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

func isPassthroughFilter(filter any) bool {
	switch filter.(type) {
	case corefilters.Passthrough, *corefilters.Passthrough:
		return true
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
	_ = workspaces.Upsert(r.workingDir, r.metricsPath)
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
	_, _ = io.Copy(target, src)
	if outWriter != nil {
		_ = outWriter.Flush()
	}
	return recorder.err
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
