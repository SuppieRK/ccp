package core

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/SuppieRK/cmdshape/internal/audit"
	"github.com/SuppieRK/cmdshape/internal/cli"
	"github.com/SuppieRK/cmdshape/internal/contracts"
	"github.com/SuppieRK/cmdshape/internal/engine"
	corefilters "github.com/SuppieRK/cmdshape/internal/filters"
	filteryaml "github.com/SuppieRK/cmdshape/internal/filters/yaml"
	"github.com/SuppieRK/cmdshape/internal/metrics"
	"github.com/SuppieRK/cmdshape/internal/projectfiles"
	"github.com/SuppieRK/cmdshape/internal/recovery"
	"github.com/SuppieRK/cmdshape/internal/replay"
	"github.com/SuppieRK/cmdshape/internal/workspaces"
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
	Stdout    string
	Stderr    string
	Decisions string
	Dispatch  string
}

// ReplayWriters receives replay artifacts as they are produced. Callers that
// can persist replay output incrementally should use this instead of retaining
// a ReplayResult for the lifetime of the invocation.
type ReplayWriters struct {
	Output    io.Writer
	Stdout    io.Writer
	Stderr    io.Writer
	Decisions io.Writer
}

type entrySink func([]engine.BufferEntry) (int, error)

type executionAudit struct {
	command   string
	tool      string
	raw       bool
	startedAt time.Time
}

type redactingWriter struct {
	writer       io.Writer
	confidential []string
	root         io.Writer
	stages       []*streamingReplacer
}

type streamingReplacer struct {
	dst     io.Writer
	old     []byte
	pending []byte
}

type streamOutput struct {
	mu             sync.Mutex
	stdoutRecorder *countingErrorWriter
	stderrRecorder *countingErrorWriter
	stdout         *redactingWriter
	stderr         *redactingWriter
}

type countingErrorWriter struct {
	writer  io.Writer
	name    string
	err     error
	written int
}

var terminalDescriptorAttached = func() bool {
	return fileIsTerminal(os.Stdin) || fileIsTerminal(os.Stdout) || fileIsTerminal(os.Stderr)
}

func NewRunnerWithOptions(opts Options) *Runner {
	projectRoot := currentProjectRoot()
	metricsPath := opts.MetricsPath
	if strings.TrimSpace(metricsPath) == "" {
		metricsPath = metrics.ProjectPath(projectRoot)
	}
	return &Runner{
		sources:     filteryaml.DefaultSources(),
		metricsPath: metricsPath,
		workingDir:  projectRoot,
		opts:        opts,
	}
}

func newConfidentialWriter(writer io.Writer, confidential []string) *redactingWriter {
	return &redactingWriter{writer: writer, confidential: confidential}
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
	execution, err := r.startExecution(args, command, false)
	if err != nil {
		return 1, err
	}

	registry, buildTiming, err := r.loadExecutionRegistry(execution.command, command.Tool)
	if err != nil {
		return 1, err
	}
	resolved := registry.Resolve(command)
	if len(r.opts.Confidential) == 0 && terminalDescriptorAttached() {
		command.Dispatch = resolved.Dispatch(command)
		audit.MustAppend("execution_terminal_fallback", map[string]any{
			"command":  execution.command,
			"tool":     command.Tool,
			"dispatch": command.Dispatch,
		})
		return r.runAttached(ctx, command, execution)
	}
	matchingArgs := slices.Clone(command.Args)
	command, err = resolved.PrepareCommand(command)
	if err != nil {
		return 1, err
	}
	command.MatchingArgs = matchingArgs
	command.Dispatch = resolved.Dispatch(command)
	state := engine.NewEngine(registry).StartResolved(command, resolved)
	cmd, stdout, stderr, err := CommandWithPipesContext(ctx, command.Args[0], command.Args[1:])
	if err != nil {
		return 1, err
	}
	if err := cmd.Start(); err != nil {
		closePipes(stdout, stderr)
		return 127, err
	}

	output := newStreamOutput(r.opts.Confidential)
	stdoutStats := &streamStats{}
	stderrStats := &streamStats{}
	stdoutWriteErr, stderrWriteErr := runConcurrently(
		func() error { return r.drainStream(stdout, state.Stdout, stdoutStats, output.writeEntries) },
		func() error { return r.drainStream(stderr, state.Stderr, stderrStats, output.writeEntries) },
	)

	exitCode, err := waitExitCode(cmd)
	_, exitWriteErr := output.writeEntries(state.Exit(exitCode))
	flushErr := output.Flush()
	outputErr := errors.Join(stdoutWriteErr, stderrWriteErr, exitWriteErr, flushErr)
	if code, runErr := filteredRunResult(ctx, err, outputErr, exitCode); runErr != nil {
		return code, runErr
	} else {
		exitCode = code
	}
	keptBytes := output.Written()
	rawBytes := stdoutStats.rawBytes + stderrStats.rawBytes
	r.maybeStoreRecovery(command, resolved, state, exitCode, rawBytes, keptBytes)
	durationMS := execution.durationMS()
	r.appendMetrics(command, filterProvenance(resolved), buildTiming, executionMetricStats{
		passthrough: isPassthroughFilter(resolved, command) || state.Passthrough(),
		exitCode:    exitCode,
		durationMS:  durationMS,
		rawBytes:    rawBytes,
		keptBytes:   keptBytes,
	})
	auditErr := execution.finish(map[string]any{
		"dispatch":    command.Dispatch,
		"exit_code":   exitCode,
		"duration_ms": durationMS,
		"raw_bytes":   rawBytes,
		"kept_bytes":  keptBytes,
	})
	if auditErr != nil {
		return auditFailureResult(exitCode, auditErr)
	}
	return exitCode, nil
}

func (r *Runner) maybeStoreRecovery(command contracts.Command, filter contracts.Filter, state *engine.State, exitCode, rawBytes, keptBytes int) {
	if exitCode == 0 || rawBytes == 0 || keptBytes >= rawBytes || state == nil || state.Passthrough() ||
		r.opts.Raw || len(r.opts.Confidential) > 0 || isPassthroughFilter(filter, command) {
		return
	}
	enabled, err := recovery.Enabled()
	if err != nil || !enabled {
		return
	}
	entries := state.RecoveryEntries()
	events := recoveryEvents(entries)
	if _, err := recovery.Store(command.ArgsForMatching(), events, exitCode); err != nil {
		audit.MustAppend("recovery_storage_error", map[string]any{
			"tool":   command.Tool,
			"reason": err.Error(),
		})
	}
}

func recoveryEvents(entries []engine.BufferEntry) []recovery.Event {
	events := make([]recovery.Event, 0, len(entries))
	for _, entry := range entries {
		if len(entry.Original) == 0 {
			continue
		}
		events = append(events, recovery.Event{
			Sequence: len(events),
			Stream:   entry.Stream,
			Data:     slices.Clone(entry.Original),
		})
	}
	return events
}

func (r *Runner) loadExecutionRegistry(auditCommand, tool string) (*engine.Registry, contracts.FilterRegistryBuildTiming, error) {
	registry, timing, err := r.loadRegistryForTool(tool)
	if err == nil {
		return registry, timing, nil
	}
	if auditErr := audit.Append("execution_registry_error", map[string]any{
		"command": auditCommand,
		"tool":    tool,
		"error":   err.Error(),
	}); auditErr != nil {
		return nil, timing, errors.Join(err, auditErr)
	}
	return engine.NewRegistry(), timing, nil
}

func filteredRunResult(ctx context.Context, waitErr, outputErr error, exitCode int) (int, error) {
	if signalCode, ok := forwardedSignalResult(ctx, exitCode); ok {
		return signalCode, outputErr
	}
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
	execution, err := r.startExecution(args, command, true)
	if err != nil {
		return 1, err
	}
	if len(r.opts.Confidential) == 0 {
		return r.runAttached(ctx, command, execution)
	}

	cmd, stdout, stderr, err := CommandWithPipesContext(ctx, command.Args[0], command.Args[1:])
	if err != nil {
		return 1, err
	}
	if err := cmd.Start(); err != nil {
		closePipes(stdout, stderr)
		return 127, err
	}

	stdoutWriteErr, stderrWriteErr := runConcurrently(
		func() error { return r.copyRawStream(stdout, os.Stdout) },
		func() error { return r.copyRawStream(stderr, os.Stderr) },
	)

	exitCode, err := waitExitCode(cmd)
	outputErr := errors.Join(stdoutWriteErr, stderrWriteErr)
	if signalCode, ok := forwardedSignalResult(ctx, exitCode); ok {
		return signalCode, outputErr
	}
	if ctx.Err() != nil {
		return 1, errors.Join(ctx.Err(), outputErr)
	}
	auditErr := execution.finish(map[string]any{
		"exit_code": exitCode,
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

func (r *Runner) runAttached(ctx context.Context, command contracts.Command, execution executionAudit) (int, error) {
	cmd := CommandAttachedContext(ctx, command.Args[0], command.Args[1:])
	if err := cmd.Start(); err != nil {
		return 127, err
	}
	exitCode, waitErr := waitExitCode(cmd)
	if signalCode, ok := forwardedSignalResult(ctx, exitCode); ok {
		return signalCode, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return 1, ctxErr
	}
	auditErr := execution.finish(map[string]any{
		"dispatch":    command.Dispatch,
		"passthrough": !execution.raw,
		"exit_code":   exitCode,
	})
	if auditErr != nil {
		return auditFailureResult(exitCode, errors.Join(waitErr, auditErr))
	}
	return exitCode, waitErr
}

func (r *Runner) startExecution(args []string, command contracts.Command, raw bool) (executionAudit, error) {
	execution := executionAudit{
		command:   r.auditCommand(command.RawInput),
		tool:      command.Tool,
		raw:       raw,
		startedAt: time.Now().UTC(),
	}
	shape := cli.DescribeExecutionShape(args)
	err := audit.Append("execution_start", map[string]any{
		"command":         execution.command,
		"tool":            execution.tool,
		"raw":             execution.raw,
		"uses_shell":      shape.UsesShell,
		"has_pipeline":    shape.HasPipeline,
		"has_chain":       shape.HasChain,
		"has_find_exec":   shape.HasFindExec,
		"has_xargs":       shape.HasXargs,
		"nested_cmdshape": shape.NestedCmdshape,
	})
	return execution, err
}

func (e executionAudit) finish(fields map[string]any) error {
	payload := map[string]any{
		"command":     e.command,
		"tool":        e.tool,
		"raw":         e.raw,
		"duration_ms": e.durationMS(),
	}
	maps.Copy(payload, fields)
	return audit.Append("execution_finish", payload)
}

func (e executionAudit) durationMS() int64 {
	return time.Since(e.startedAt).Milliseconds()
}

func runConcurrently(first, second func() error) (firstErr, secondErr error) {
	var wg sync.WaitGroup
	wg.Go(func() {
		firstErr = first()
	})
	wg.Go(func() {
		secondErr = second()
	})
	wg.Wait()
	return firstErr, secondErr
}

func (r *Runner) Verify(args []string, stdout, stderr io.Reader) (string, error) {
	events, err := replay.ReadEventReaders(stdout, stderr)
	if err != nil {
		return "", err
	}
	result, err := r.ReplayWithExitCode(args, events, 0)
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

func (r *Runner) ReplayWithExitCode(args []string, events []replay.Event, exitCode int) (ReplayResult, error) {
	return r.ReplaySequenceWithExitCode(args, func(yield func(replay.Event, error) bool) {
		for _, event := range events {
			if !yield(event, nil) {
				return
			}
		}
	}, exitCode)
}

func (r *Runner) ReplaySequenceWithExitCode(args []string, events iter.Seq2[replay.Event, error], exitCode int) (ReplayResult, error) {
	collector := &replayCollector{}
	dispatch, err := r.replaySequenceWithExitCode(args, events, exitCode, collector)
	if err != nil {
		return ReplayResult{}, err
	}
	return ReplayResult{
		Output:    collector.output.String(),
		Stdout:    collector.stdout.String(),
		Stderr:    collector.stderr.String(),
		Decisions: collector.decisions.String(),
		Dispatch:  dispatch,
	}, nil
}

// ReplaySequenceToWritersWithExitCode replays an event sequence without
// retaining the generated artifacts in memory. The existing ReplayResult APIs
// remain available for callers that need an in-memory result.
func (r *Runner) ReplaySequenceToWritersWithExitCode(
	args []string,
	events iter.Seq2[replay.Event, error],
	exitCode int,
	writers ReplayWriters,
) (string, error) {
	collector := newStreamingReplayCollector(writers, r.opts.Confidential)
	dispatch, replayErr := r.replaySequenceWithExitCode(args, events, exitCode, collector)
	return dispatch, errors.Join(replayErr, collector.Flush())
}

type replayRecorder interface {
	recordInput(replay.Event, contracts.Action, []engine.BufferEntry)
	recordExit(contracts.Action, []engine.BufferEntry)
	Err() error
	OutputBytes() int
}

func (r *Runner) replaySequenceWithExitCode(
	args []string,
	events iter.Seq2[replay.Event, error],
	exitCode int,
	collector replayRecorder,
) (string, error) {
	command, err := ParseCommandArgs(args)
	if err != nil {
		return "", err
	}
	auditCommand := r.auditCommand(command.RawInput)
	if err := audit.Append("verify_start", map[string]any{
		"command": auditCommand,
		"tool":    command.Tool,
	}); err != nil {
		return "", err
	}

	registry, _, err := r.loadRegistryForTool(command.Tool)
	if err != nil {
		return "", errors.Join(err, audit.Append("verify_registry_error", map[string]any{
			"command": auditCommand,
			"tool":    command.Tool,
			"error":   err.Error(),
		}))
	}
	resolved := registry.Resolve(command)
	matchingArgs := slices.Clone(command.Args)
	command, err = resolved.PrepareCommand(command)
	if err != nil {
		return "", err
	}
	command.MatchingArgs = matchingArgs
	command.Dispatch = resolved.Dispatch(command)
	state := engine.NewEngine(registry).StartResolved(command, resolved)
	for event, eventErr := range events {
		if eventErr != nil {
			return "", eventErr
		}
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
		if err := collector.Err(); err != nil {
			return "", err
		}
	}
	exitAction, exitEntries := state.ExitAction(exitCode)
	collector.recordExit(exitAction, exitEntries)
	if err := collector.Err(); err != nil {
		return "", err
	}
	if err := audit.Append("verify_finish", map[string]any{
		"command":      auditCommand,
		"tool":         command.Tool,
		"dispatch":     command.Dispatch,
		"output_bytes": collector.OutputBytes(),
	}); err != nil {
		return "", err
	}
	return command.Dispatch, nil
}

func (r *Runner) loadRegistry() (*engine.Registry, contracts.FilterRegistryBuildTiming, error) {
	startedAt := time.Now()
	registry := engine.NewRegistry()
	filters, timing, err := filteryaml.LoadRegistryFiltersFromSourcesWithTiming(r.sources)
	timing.DurationMS = time.Since(startedAt).Milliseconds()
	if err != nil {
		return nil, timing, err
	}
	registry.RegisterAll(filters)
	timing.DurationMS = time.Since(startedAt).Milliseconds()
	return registry, timing, nil
}

func (r *Runner) loadRegistryForTool(tool string) (*engine.Registry, contracts.FilterRegistryBuildTiming, error) {
	startedAt := time.Now()
	registry := engine.NewRegistry()
	filters, timing, err := filteryaml.LoadExecutionFilterFromSourcesWithTiming(r.sources, tool)
	timing.DurationMS = time.Since(startedAt).Milliseconds()
	if err != nil {
		return nil, timing, err
	}
	registry.RegisterAll(filters)
	timing.DurationMS = time.Since(startedAt).Milliseconds()
	return registry, timing, nil
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
	var sinkErr error
	for {
		record, err := replay.ReadStreamRecord(reader)
		if len(record) > 0 {
			if stats != nil {
				stats.rawBytes += len(record)
			}
			written, writeErr := sink(consume(string(record)))
			r.recordSinkResult(stats, written, writeErr, &sinkErr)
		}
		if err != nil {
			if errors.Is(err, replay.ErrStreamRecordLimit) {
				continue
			}
			return errors.Join(sinkErr, wrapStreamReadError(err))
		}
	}
}

func (r *Runner) recordSinkResult(stats *streamStats, written int, err error, sinkErr *error) {
	if stats != nil {
		stats.keptBytes += written
	}
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

func newStreamOutput(confidential []string) *streamOutput {
	stdoutRecorder := &countingErrorWriter{writer: os.Stdout, name: "stdout"}
	stderrRecorder := &countingErrorWriter{writer: os.Stderr, name: "stderr"}
	return &streamOutput{
		stdoutRecorder: stdoutRecorder,
		stderrRecorder: stderrRecorder,
		stdout:         &redactingWriter{writer: stdoutRecorder, confidential: confidential},
		stderr:         &redactingWriter{writer: stderrRecorder, confidential: confidential},
	}
}

func (o *streamOutput) writeEntries(entries []engine.BufferEntry) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	before := o.writtenLocked()
	for _, entry := range entries {
		writer := o.stdout
		if entry.Stream == contracts.StreamStderr {
			writer = o.stderr
		}
		if _, err := writer.Write([]byte(entry.Line)); err != nil {
			return o.writtenLocked() - before, err
		}
	}
	return o.writtenLocked() - before, nil
}

func (o *streamOutput) Flush() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return errors.Join(o.stdout.Flush(), o.stderr.Flush(), o.stdoutRecorder.err, o.stderrRecorder.err)
}

func (o *streamOutput) Written() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.writtenLocked()
}

func (o *streamOutput) writtenLocked() int {
	return o.stdoutRecorder.written + o.stderrRecorder.written
}

type replayCollector struct {
	output    bytes.Buffer
	stdout    bytes.Buffer
	stderr    bytes.Buffer
	decisions bytes.Buffer
}

func (c *replayCollector) Err() error { return nil }

func (c *replayCollector) OutputBytes() int { return c.output.Len() }

func (c *replayCollector) writeEntries(entries []engine.BufferEntry) int {
	written := 0
	for _, entry := range entries {
		written += len(entry.Line)
		_, _ = c.output.WriteString(entry.Line)
		switch entry.Stream {
		case contracts.StreamStderr:
			_, _ = c.stderr.WriteString(entry.Line)
		default:
			_, _ = c.stdout.WriteString(entry.Line)
		}
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

type streamingReplayCollector struct {
	output      *redactingWriter
	stdout      *redactingWriter
	stderr      *redactingWriter
	decisions   *redactingWriter
	outputBytes int
	err         error
}

func newStreamingReplayCollector(writers ReplayWriters, confidential []string) *streamingReplayCollector {
	writerOrDiscard := func(writer io.Writer) io.Writer {
		if writer == nil {
			return io.Discard
		}
		return writer
	}
	return &streamingReplayCollector{
		output:    newConfidentialWriter(writerOrDiscard(writers.Output), confidential),
		stdout:    newConfidentialWriter(writerOrDiscard(writers.Stdout), confidential),
		stderr:    newConfidentialWriter(writerOrDiscard(writers.Stderr), confidential),
		decisions: newConfidentialWriter(writerOrDiscard(writers.Decisions), confidential),
	}
}

func (c *streamingReplayCollector) Err() error { return c.err }

func (c *streamingReplayCollector) OutputBytes() int { return c.outputBytes }

func (c *streamingReplayCollector) recordInput(event replay.Event, action contracts.Action, emitted []engine.BufferEntry) {
	c.writeDecision(labelForInputAction(action), event.Line)
	c.writeEntries(emitted)
	if action.Kind == contracts.ActionReplace {
		for _, entry := range emitted {
			c.writeDecision("<emit>", entry.Line)
		}
	}
}

func (c *streamingReplayCollector) recordExit(_ contracts.Action, emitted []engine.BufferEntry) {
	c.writeEntries(emitted)
}

func (c *streamingReplayCollector) writeEntries(entries []engine.BufferEntry) {
	for _, entry := range entries {
		if c.err != nil {
			return
		}
		c.outputBytes += len(entry.Line)
		c.writeString(c.output, entry.Line)
		switch entry.Stream {
		case contracts.StreamStderr:
			c.writeString(c.stderr, entry.Line)
		default:
			c.writeString(c.stdout, entry.Line)
		}
	}
}

func (c *streamingReplayCollector) writeDecision(label, line string) {
	for _, part := range splitDecisionLines(line) {
		if c.err != nil {
			return
		}
		text := strings.TrimSuffix(part, "\n")
		c.writeString(c.decisions, fmt.Sprintf("%-10s| %s\n", label, text))
	}
}

func (c *streamingReplayCollector) writeString(writer io.Writer, value string) {
	if c.err != nil {
		return
	}
	_, c.err = io.WriteString(writer, value)
}

func (c *streamingReplayCollector) Flush() error {
	return errors.Join(c.err, c.output.Flush(), c.stdout.Flush(), c.stderr.Flush(), c.decisions.Flush())
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
		if cmd.ProcessState != nil && cmd.ProcessState.Success() {
			return 0, nil
		}
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			if code, ok := nativeExitCode(exitErr); ok {
				return code, nil
			}
			return 1, err
		}
		return 1, err
	}
	return 0, nil
}

func forwardedSignalResult(ctx context.Context, exitCode int) (int, bool) {
	cause, ok := errors.AsType[executionSignal](context.Cause(ctx))
	if !ok {
		return 0, false
	}
	if isHardKillExitCode(exitCode) {
		return executionSignalExitCode(cause.signal), true
	}
	return exitCode, true
}

// ForwardedSignalExitCode returns the native shell code for an OS signal
// forwarded by cmdshape. Ordinary context cancellation is not a signal.
func ForwardedSignalExitCode(ctx context.Context, exitCode int) (int, bool) {
	return forwardedSignalResult(ctx, exitCode)
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

func filterProvenance(filter any) contracts.FilterProvenance {
	if f, ok := filter.(contracts.ProvenanceFilter); ok {
		return f.FilterProvenance()
	}
	return contracts.FilterProvenance{}
}

type executionMetricStats struct {
	passthrough bool
	exitCode    int
	durationMS  int64
	rawBytes    int
	keptBytes   int
}

func (r *Runner) appendMetrics(command contracts.Command, provenance contracts.FilterProvenance, buildTiming contracts.FilterRegistryBuildTiming, stats executionMetricStats) {
	if r.opts.Raw {
		return
	}
	if !shouldRecordMetrics(command) {
		return
	}
	metric := metrics.RunMetric{
		Timestamp:             time.Now().UTC(),
		Command:               r.auditCommand(command.RawInput),
		Tool:                  command.Tool,
		Dispatch:              command.Dispatch,
		RawBytes:              stats.rawBytes,
		KeptBytes:             stats.keptBytes,
		ExitCode:              stats.exitCode,
		DurationMS:            stats.durationMS,
		Passthrough:           stats.passthrough,
		FilterSourceKind:      provenance.SourceKind,
		FilterPath:            provenance.Path,
		FilterHash:            provenance.Hash,
		RegistryBuildRecorded: true,
		RegistryBuildMS:       buildTiming.DurationMS,
		RegistrySources:       metricRegistrySources(buildTiming.Sources),
	}
	projectRoot, contained := defaultProjectMetricsRoot(r.workingDir, r.metricsPath)
	var err error
	if contained {
		err = metrics.AppendProject(projectRoot, r.metricsPath, metric)
	} else {
		err = metrics.Append(r.metricsPath, metric)
	}
	if err != nil {
		audit.MustAppend("metrics_storage_error", map[string]any{
			"tool":   command.Tool,
			"reason": err.Error(),
		})
		return
	}
	if strings.TrimSpace(r.workingDir) == "" {
		return
	}
	path, err := workspaces.DefaultPath()
	if err != nil {
		return
	}
	workingDir := r.workingDir
	metricsPath := r.metricsPath
	if contained {
		if validateErr := projectfiles.ValidateRegularFileBeneath(projectRoot, metricsPath); validateErr != nil {
			audit.MustAppend("metrics_storage_error", map[string]any{
				"tool":   command.Tool,
				"reason": validateErr.Error(),
			})
			return
		}
		canonicalMetricsPath, canonicalErr := projectfiles.CanonicalPathBeneath(projectRoot, metricsPath)
		if canonicalErr != nil {
			return
		}
		workingDir = filepath.Dir(filepath.Dir(canonicalMetricsPath))
		metricsPath = canonicalMetricsPath
	}
	_ = workspaces.UpsertPath(path, workingDir, metricsPath)
}

func defaultProjectMetricsRoot(workingDir, metricsPath string) (string, bool) {
	if strings.TrimSpace(workingDir) == "" || strings.TrimSpace(metricsPath) == "" {
		return "", false
	}
	root, err := filepath.Abs(filepath.Clean(workingDir))
	if err != nil {
		return "", false
	}
	path, err := filepath.Abs(filepath.Clean(metricsPath))
	if err != nil {
		return "", false
	}
	return root, path == metrics.ProjectPath(root)
}

func metricRegistrySources(sources []contracts.FilterSourceBuildTiming) []metrics.RegistrySourceBuildMetric {
	out := make([]metrics.RegistrySourceBuildMetric, 0, len(sources))
	for _, source := range sources {
		out = append(out, metrics.RegistrySourceBuildMetric{
			SourceKind:  source.SourceKind,
			SourceDir:   source.SourceDir,
			Definitions: source.Definitions,
			Compiled:    source.Compiled,
			DurationMS:  source.DurationMS,
			Error:       source.Error,
		})
	}
	return out
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

func currentProjectRoot() string {
	cwd := currentWorkingDir()
	if cwd == "" {
		return ""
	}
	root, err := projectfiles.ResolveProjectRoot(cwd)
	if err != nil {
		return ""
	}
	return root
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

func (w *countingErrorWriter) Write(p []byte) (int, error) {
	if len(p) == 0 || w.writer == nil || w.err != nil {
		return len(p), nil
	}
	written, err := w.writer.Write(p)
	w.written += written
	if err != nil {
		w.err = fmt.Errorf("write %s: %w", w.name, err)
		return len(p), nil
	}
	if written != len(p) {
		w.err = fmt.Errorf("write %s: %w", w.name, io.ErrShortWrite)
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
	w.initialize()
	if _, err := w.root.Write(p); err != nil {
		return len(p), err
	}
	return len(p), nil
}

func (w *redactingWriter) Flush() error {
	if w.writer == nil {
		return nil
	}
	w.initialize()
	for _, stage := range w.stages {
		if err := stage.Flush(); err != nil {
			return err
		}
	}
	return nil
}

func (w *redactingWriter) initialize() {
	if w.root != nil {
		return
	}
	w.root = w.writer
	valid := make([]string, 0, len(w.confidential))
	for _, token := range w.confidential {
		if token != "" {
			valid = append(valid, token)
		}
	}
	w.stages = make([]*streamingReplacer, len(valid))
	for index := len(valid) - 1; index >= 0; index-- {
		stage := &streamingReplacer{dst: w.root, old: []byte(valid[index])}
		w.stages[index] = stage
		w.root = stage
	}
}

func (w *streamingReplacer) Write(p []byte) (int, error) {
	w.pending = append(w.pending, p...)
	if err := w.drain(false); err != nil {
		return len(p), err
	}
	return len(p), nil
}

func (w *streamingReplacer) Flush() error {
	return w.drain(true)
}

func (w *streamingReplacer) drain(final bool) error {
	for {
		if index := bytes.Index(w.pending, w.old); index >= 0 {
			if err := writeAll(w.dst, w.pending[:index]); err != nil {
				return err
			}
			if err := writeAll(w.dst, []byte("***")); err != nil {
				return err
			}
			w.pending = w.pending[index+len(w.old):]
			continue
		}
		keep := len(w.old) - 1
		if final {
			keep = 0
		}
		flush := len(w.pending) - min(len(w.pending), keep)
		if flush == 0 {
			return nil
		}
		if err := writeAll(w.dst, w.pending[:flush]); err != nil {
			return err
		}
		w.pending = bytes.Clone(w.pending[flush:])
		return nil
	}
}

func writeAll(dst io.Writer, p []byte) error {
	for len(p) > 0 {
		written, err := dst.Write(p)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		p = p[written:]
	}
	return nil
}
