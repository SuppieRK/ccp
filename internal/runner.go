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
	"slices"
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
	"go-command-compression-proxy/internal/projectfiles"
	"go-command-compression-proxy/internal/recovery"
	"go-command-compression-proxy/internal/replay"
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
	Stdout    string
	Stderr    string
	Decisions string
	Dispatch  string
}

type entrySink func([]engine.BufferEntry) (int, error)

type redactingWriter struct {
	writer       io.Writer
	confidential []string
	buf          []byte
}

var terminalDescriptorAttached = func() bool {
	return fileIsTerminal(os.Stdin) || fileIsTerminal(os.Stdout) || fileIsTerminal(os.Stderr)
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

	registry, buildTiming, err := r.loadExecutionRegistry(auditCommand, command.Tool)
	if err != nil {
		return 1, err
	}
	resolved := registry.Resolve(command)
	if len(r.opts.Confidential) == 0 && terminalDescriptorAttached() {
		command.Dispatch = resolved.Dispatch(command)
		audit.MustAppend("execution_terminal_fallback", map[string]any{
			"command":  auditCommand,
			"tool":     command.Tool,
			"dispatch": command.Dispatch,
		})
		return r.runAttached(ctx, command, startedAt, false)
	}
	matchingArgs := slices.Clone(command.Args)
	command, err = resolved.PrepareCommand(command)
	if err != nil {
		return 1, err
	}
	command.MatchingArgs = matchingArgs
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
	r.maybeStoreRecovery(command, resolved, state, exitCode, rawBytes, keptBytes)
	r.appendMetrics(command, filterProvenance(resolved), buildTiming, isPassthroughFilter(resolved, command) || state.Passthrough(), exitCode, time.Since(startedAt).Milliseconds(), rawBytes, keptBytes)
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
	events := make([]recovery.Event, 0, len(entries))
	for _, entry := range entries {
		if len(entry.Original) == 0 {
			continue
		}
		events = append(events, recovery.Event{
			Sequence: int(entry.Sequence),
			Stream:   entry.Stream,
			Data:     slices.Clone(entry.Original),
		})
	}
	if _, err := recovery.Store(command.ArgsForMatching(), events, exitCode); err != nil {
		audit.MustAppend("recovery_storage_error", map[string]any{
			"tool":   command.Tool,
			"reason": err.Error(),
		})
	}
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
	if len(r.opts.Confidential) == 0 {
		return r.runAttached(ctx, command, startedAt, true)
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

func (r *Runner) runAttached(ctx context.Context, command contracts.Command, startedAt time.Time, raw bool) (int, error) {
	cmd := CommandAttachedContext(ctx, command.Args[0], command.Args[1:])
	if err := cmd.Start(); err != nil {
		return 127, err
	}
	exitCode, waitErr := waitExitCode(cmd)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return 1, ctxErr
	}
	auditErr := audit.Append("execution_finish", map[string]any{
		"command":     r.auditCommand(command.RawInput),
		"tool":        command.Tool,
		"dispatch":    command.Dispatch,
		"raw":         raw,
		"passthrough": !raw,
		"exit_code":   exitCode,
		"duration_ms": time.Since(startedAt).Milliseconds(),
	})
	if auditErr != nil {
		return auditFailureResult(exitCode, errors.Join(waitErr, auditErr))
	}
	return exitCode, waitErr
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
	command, err := ParseCommandArgs(args)
	if err != nil {
		return ReplayResult{}, err
	}
	auditCommand := r.auditCommand(command.RawInput)
	if err := audit.Append("verify_start", map[string]any{
		"command": auditCommand,
		"tool":    command.Tool,
	}); err != nil {
		return ReplayResult{}, err
	}

	registry, _, err := r.loadRegistryForTool(command.Tool)
	if err != nil {
		return ReplayResult{}, errors.Join(err, audit.Append("verify_registry_error", map[string]any{
			"command": auditCommand,
			"tool":    command.Tool,
			"error":   err.Error(),
		}))
	}
	resolved := registry.Resolve(command)
	matchingArgs := slices.Clone(command.Args)
	command, err = resolved.PrepareCommand(command)
	if err != nil {
		return ReplayResult{}, err
	}
	command.MatchingArgs = matchingArgs
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
	if err := audit.Append("verify_finish", map[string]any{
		"command":      auditCommand,
		"tool":         command.Tool,
		"dispatch":     command.Dispatch,
		"output_bytes": len(collector.output.String()),
	}); err != nil {
		return ReplayResult{}, err
	}
	return ReplayResult{
		Output:    collector.output.String(),
		Stdout:    collector.stdout.String(),
		Stderr:    collector.stderr.String(),
		Decisions: collector.decisions.String(),
		Dispatch:  command.Dispatch,
	}, nil
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

type replayCollector struct {
	output    bytes.Buffer
	stdout    bytes.Buffer
	stderr    bytes.Buffer
	decisions bytes.Buffer
}

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

func filterProvenance(filter any) contracts.FilterProvenance {
	if f, ok := filter.(contracts.ProvenanceFilter); ok {
		return f.FilterProvenance()
	}
	return contracts.FilterProvenance{}
}

func (r *Runner) appendMetrics(command contracts.Command, provenance contracts.FilterProvenance, buildTiming contracts.FilterRegistryBuildTiming, passthrough bool, exitCode int, durationMS int64, rawBytes, keptBytes int) {
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
		RawBytes:              rawBytes,
		KeptBytes:             keptBytes,
		ExitCode:              exitCode,
		DurationMS:            durationMS,
		Passthrough:           passthrough,
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
	return root, path == filepath.Join(root, ".ccp", "gain.db")
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
