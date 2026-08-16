package lifecycle

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	core "github.com/SuppieRK/cmdshape/internal"
	"github.com/SuppieRK/cmdshape/internal/audit"
	"github.com/SuppieRK/cmdshape/internal/contracts"
	"github.com/SuppieRK/cmdshape/internal/replay"
)

const (
	captureStdoutFileName       = replay.StdoutFileName
	captureStderrFileName       = replay.StderrFileName
	captureOutputFileName       = replay.OutputFileName
	captureOutputStdoutFileName = replay.OutputStdoutFileName
	captureOutputStderrFileName = replay.OutputStderrFileName
)

type captureVerifier interface {
	ReplaySequenceToWritersWithExitCode(args []string, events iter.Seq2[replay.Event, error], exitCode int, writers core.ReplayWriters) (string, error)
}

var newCaptureRunner = func(confidential []string) captureVerifier {
	return core.NewRunnerWithOptions(core.Options{Confidential: confidential})
}

func RunCapture(args []string) error {
	fs := newLifecycleFlagSet("capture")
	dirFlag := fs.String("dir", "", "directory where capture artifacts are written")
	confidentialFlag := fs.String("confidential", "", "comma-separated literal values to redact from captured argv and output")
	setLifecycleUsage(
		fs,
		"capture native stdout/stderr and replay cmdshape output for local filter iteration",
		[]string{"cmdshape capture [--dir <path>] -- <command> [args...]"},
		"capture writes command.yaml, sequenced native streams, output.txt and stream-aware output expectations, decisions.txt, and verify-dispatch.txt.",
		"when --dir is omitted, capture writes to the current working directory.",
		"stdout.txt and stderr.txt use sequenced 00000| prefixes so replay preserves cross-stream ordering.",
		"capture runs the command natively once, then replays the captured streams through the current YAML runtime.",
		"non-zero command exits still write artifacts so the failure can be iterated on locally.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	confidential := captureConfidentialValues(*confidentialFlag)
	if err != nil {
		return recordCaptureFailure(nil, "", confidential, "parse_flags", err)
	}
	if handled {
		audit.MustAppend("capture_invocation_finish", map[string]any{
			"success": true,
			"stage":   "help",
		})
		return nil
	}

	commandArgs := fs.Args()
	dirValue := strings.TrimSpace(*dirFlag)
	if len(commandArgs) == 0 {
		return recordCaptureFailure(commandArgs, dirValue, confidential, "validate_flags", errors.New("missing command after '--'"))
	}

	captureDir, err := resolveCaptureDir(dirValue, commandArgs[0])
	if err != nil {
		return recordCaptureFailure(commandArgs, dirValue, confidential, "resolve_dir", err)
	}
	return executeCapture(commandArgs, captureDir, confidential)
}

func recordCaptureFailure(commandArgs []string, dir string, confidential []string, stage string, err error) error {
	audit.MustAppend("capture_invocation_finish", map[string]any{
		"command": strings.Join(redactCaptureArgs(commandArgs, confidential), " "),
		"dir":     redactCaptureText(dir, confidential),
		"success": false,
		"stage":   stage,
		"error":   redactCaptureText(err.Error(), confidential),
	})
	return err
}

func executeCapture(commandArgs []string, captureDir string, confidential []string) error {
	auditCommand := strings.Join(redactCaptureArgs(commandArgs, confidential), " ")
	recordFailure := func(stage string, err error) error {
		return recordCaptureFailure(commandArgs, captureDir, confidential, stage, err)
	}
	audit.MustAppend("capture_invocation_start", map[string]any{
		"command": auditCommand,
		"dir":     redactCaptureText(captureDir, confidential),
	})

	recording, exitCode, err := runNativeCaptureStaged(commandArgs)
	if err != nil {
		return recordFailure("native_exec", err)
	}
	defer recording.cleanup()
	if err := ensureCaptureDirectory(captureDir); err != nil {
		return recordFailure("mkdir", err)
	}

	paths := replay.FixturePaths(captureDir)
	commandPath := paths[replay.CommandFileName]
	stdoutPath := paths[replay.StdoutFileName]
	stderrPath := paths[replay.StderrFileName]
	outputPath := paths[replay.OutputFileName]
	outputStdoutPath := paths[replay.OutputStdoutFileName]
	outputStderrPath := paths[replay.OutputStderrFileName]
	decisionsPath := paths[replay.DecisionsFileName]
	dispatchPath := paths[replay.VerifyDispatchFileName]
	if err := tightenCaptureTargets([]string{commandPath, stdoutPath, stderrPath, outputPath, outputStdoutPath, outputStderrPath, decisionsPath, dispatchPath}); err != nil {
		return recordFailure("tighten_targets", err)
	}
	storedArgs := redactCaptureArgs(commandArgs, confidential)
	if err := replay.WriteCommandWithExitCodeMode(commandPath, storedArgs, exitCode, len(confidential) > 0, 0o600); err != nil {
		return recordFailure("write_command", err)
	}
	if err := writeCapturedStreams(recording.stdoutPath, recording.stderrPath, stdoutPath, stderrPath, confidential); err != nil {
		return recordFailure("write_streams", err)
	}

	stdoutEvents, err := os.Open(stdoutPath)
	if err != nil {
		return recordFailure("replay_output", err)
	}
	defer func() { _ = stdoutEvents.Close() }()
	stderrEvents, err := os.Open(stderrPath)
	if err != nil {
		return recordFailure("replay_output", err)
	}
	defer func() { _ = stderrEvents.Close() }()
	stagedArtifacts, err := newStagedCaptureArtifacts(captureDir)
	if err != nil {
		return recordFailure("stage_output", err)
	}
	defer stagedArtifacts.cleanup()
	dispatch, err := newCaptureRunner(confidential).ReplaySequenceToWritersWithExitCode(
		commandArgs,
		replay.ReadMergedEventReaders(stdoutEvents, stderrEvents),
		exitCode,
		stagedArtifacts.writers(),
	)
	if err != nil {
		return recordFailure("replay_output", err)
	}
	if err := stagedArtifacts.closeAndSync(); err != nil {
		return recordFailure("stage_output", err)
	}
	if err := stagedArtifacts.promote(map[string]string{
		replay.OutputFileName:       outputPath,
		replay.OutputStdoutFileName: outputStdoutPath,
		replay.OutputStderrFileName: outputStderrPath,
		replay.DecisionsFileName:    decisionsPath,
	}); err != nil {
		return recordFailure("write_output", err)
	}
	if err := replay.WriteArtifact(dispatchPath, []byte(redactCaptureText(dispatch, confidential)+"\n"), 0o600); err != nil {
		return recordFailure("write_dispatch", err)
	}
	outputInfo, err := os.Stat(outputPath)
	if err != nil {
		return recordFailure("stat_output", err)
	}

	audit.MustAppend("capture_invocation_finish", map[string]any{
		"command":            auditCommand,
		"dir":                redactCaptureText(captureDir, confidential),
		"command_path":       redactCaptureText(commandPath, confidential),
		"stdout_path":        redactCaptureText(stdoutPath, confidential),
		"stderr_path":        redactCaptureText(stderrPath, confidential),
		"output_path":        redactCaptureText(outputPath, confidential),
		"output_stdout_path": redactCaptureText(outputStdoutPath, confidential),
		"output_stderr_path": redactCaptureText(outputStderrPath, confidential),
		"exit_code":          exitCode,
		"stdout_bytes":       recording.stdoutBytes,
		"stderr_bytes":       recording.stderrBytes,
		"output_bytes":       outputInfo.Size(),
		"success":            true,
	})

	if exitCode != 0 {
		return captureExitError{code: exitCode}
	}
	return nil
}

func resolveCaptureDir(dir, _ string) (string, error) {
	if strings.TrimSpace(dir) != "" {
		return filepath.Abs(dir)
	}
	return os.Getwd()
}

func ensureCaptureDirectory(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("capture path %q is not a directory", path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func tightenCaptureTargets(paths []string) error {
	for _, path := range paths {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refuse non-regular capture target %q", path)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func captureConfidentialValues(raw string) []string {
	values := make([]string, 0, 4)
	for value := range strings.SplitSeq(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func redactCaptureArgs(args, confidential []string) []string {
	out := make([]string, len(args))
	for index, arg := range args {
		out[index] = redactCaptureText(arg, confidential)
	}
	return out
}

func redactCaptureText(value string, confidential []string) string {
	for _, secret := range confidential {
		value = strings.ReplaceAll(value, secret, "***")
	}
	return value
}

type attributedCaptureWriter interface {
	WriteAttributed(captureAttribution, []byte) error
	Flush() error
}

type captureAttribution struct {
	sequence int
	stream   contracts.Stream
}

type captureAttributionSpan struct {
	attribution captureAttribution
	bytes       int
}

type attributedCaptureReplacer struct {
	dst     attributedCaptureWriter
	old     []byte
	pending []byte
	head    int
	spans   []captureAttributionSpan
}

func (w *attributedCaptureReplacer) WriteAttributed(attribution captureAttribution, payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	w.pending = append(w.pending, payload...)
	if count := len(w.spans); count > 0 && w.spans[count-1].attribution == attribution {
		w.spans[count-1].bytes += len(payload)
	} else {
		w.spans = append(w.spans, captureAttributionSpan{attribution: attribution, bytes: len(payload)})
	}
	return w.drain(false)
}

func (w *attributedCaptureReplacer) Flush() error {
	return w.drain(true)
}

func (w *attributedCaptureReplacer) drain(final bool) error {
	for {
		remaining := w.pending[w.head:]
		if index := bytes.Index(remaining, w.old); index >= 0 {
			if err := w.consume(index, true); err != nil {
				return err
			}
			attribution := w.spans[0].attribution
			if err := w.consume(len(w.old), false); err != nil {
				return err
			}
			if err := w.dst.WriteAttributed(attribution, []byte("***")); err != nil {
				return err
			}
			continue
		}

		keep := len(w.old) - 1
		if final {
			keep = 0
		}
		flush := len(remaining) - min(len(remaining), keep)
		if err := w.consume(flush, true); err != nil {
			return err
		}
		w.compact()
		return nil
	}
}

func (w *attributedCaptureReplacer) consume(count int, emit bool) error {
	remaining := count
	position := w.head
	for remaining > 0 {
		span := &w.spans[0]
		take := min(remaining, span.bytes)
		if emit {
			if err := w.dst.WriteAttributed(span.attribution, w.pending[position:position+take]); err != nil {
				return err
			}
		}
		position += take
		w.head += take
		remaining -= take
		span.bytes -= take
		if span.bytes == 0 {
			w.spans = w.spans[1:]
		}
	}
	return nil
}

func (w *attributedCaptureReplacer) compact() {
	if w.head == 0 {
		return
	}
	w.pending = bytes.Clone(w.pending[w.head:])
	w.head = 0
}

type capturedEventWriter struct {
	stdout  *bufio.Writer
	stderr  *bufio.Writer
	pending []capturedEventBuffer
	head    int
}

type capturedEventBuffer struct {
	attribution captureAttribution
	payload     []byte
}

func (w *capturedEventWriter) Register(event replay.Event) error {
	attribution := captureAttribution{sequence: event.Sequence, stream: event.Stream}
	if count := len(w.pending); count > w.head && w.pending[count-1].attribution.sequence >= event.Sequence {
		return fmt.Errorf("capture sequence is not increasing: %05d", event.Sequence)
	}
	w.pending = append(w.pending, capturedEventBuffer{attribution: attribution})
	return nil
}

func (w *capturedEventWriter) WriteAttributed(attribution captureAttribution, payload []byte) error {
	if err := w.flushBefore(attribution.sequence); err != nil {
		return err
	}
	if w.head >= len(w.pending) || w.pending[w.head].attribution != attribution {
		return fmt.Errorf("capture output references unknown event %05d", attribution.sequence)
	}
	w.pending[w.head].payload = append(w.pending[w.head].payload, payload...)
	return nil
}

func (w *capturedEventWriter) flushBefore(sequence int) error {
	for w.head < len(w.pending) && w.pending[w.head].attribution.sequence < sequence {
		if err := w.writeEvent(w.pending[w.head]); err != nil {
			return err
		}
		w.pending[w.head] = capturedEventBuffer{}
		w.head++
	}
	if w.head > 0 && w.head*2 >= len(w.pending) {
		w.pending = append(w.pending[:0], w.pending[w.head:]...)
		w.head = 0
	}
	return nil
}

func (w *capturedEventWriter) writeEvent(buffer capturedEventBuffer) error {
	dst := w.stdout
	if buffer.attribution.stream == contracts.StreamStderr {
		dst = w.stderr
	}
	event := replay.Event{
		Sequence: buffer.attribution.sequence,
		Stream:   buffer.attribution.stream,
		Line:     string(buffer.payload),
	}
	return replay.WriteSequencedEvent(dst, event)
}

func (w *capturedEventWriter) Flush() error {
	for w.head < len(w.pending) {
		if err := w.writeEvent(w.pending[w.head]); err != nil {
			return err
		}
		w.pending[w.head] = capturedEventBuffer{}
		w.head++
	}
	w.pending = nil
	w.head = 0
	return errors.Join(w.stdout.Flush(), w.stderr.Flush())
}

func newAttributedCaptureWriter(dst attributedCaptureWriter, confidential []string) (attributedCaptureWriter, []*attributedCaptureReplacer) {
	valid := make([]string, 0, len(confidential))
	for _, secret := range confidential {
		if secret != "" {
			valid = append(valid, secret)
		}
	}
	stages := make([]*attributedCaptureReplacer, len(valid))
	root := dst
	for index := len(valid) - 1; index >= 0; index-- {
		stage := &attributedCaptureReplacer{dst: root, old: []byte(valid[index])}
		stages[index] = stage
		root = stage
	}
	return root, stages
}

func writeCapturedStreams(stdoutSrcPath, stderrSrcPath, stdoutDstPath, stderrDstPath string, confidential []string) (err error) {
	stdoutSrc, err := os.Open(stdoutSrcPath)
	if err != nil {
		return err
	}
	defer closeWithErr(stdoutSrc, &err)
	stderrSrc, err := os.Open(stderrSrcPath)
	if err != nil {
		return err
	}
	defer closeWithErr(stderrSrc, &err)
	stdoutDst, err := os.OpenFile(stdoutDstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer closeWithErr(stdoutDst, &err)
	stderrDst, err := os.OpenFile(stderrDstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer closeWithErr(stderrDst, &err)

	finalWriter := &capturedEventWriter{stdout: bufio.NewWriter(stdoutDst), stderr: bufio.NewWriter(stderrDst)}
	root, stages := newAttributedCaptureWriter(finalWriter, confidential)
	for event, readErr := range replay.ReadMergedEventReaders(stdoutSrc, stderrSrc) {
		if readErr != nil {
			return readErr
		}
		if err := finalWriter.Register(event); err != nil {
			return err
		}
		attribution := captureAttribution{sequence: event.Sequence, stream: event.Stream}
		if err := root.WriteAttributed(attribution, []byte(event.Line)); err != nil {
			return err
		}
	}
	for _, stage := range stages {
		if err := stage.Flush(); err != nil {
			return err
		}
	}
	if err := finalWriter.Flush(); err != nil {
		return err
	}
	return errors.Join(stdoutDst.Sync(), stderrDst.Sync())
}

type stagedCaptureArtifacts struct {
	dir   string
	files map[string]*os.File
}

func newStagedCaptureArtifacts(captureDir string) (*stagedCaptureArtifacts, error) {
	dir, err := os.MkdirTemp(captureDir, ".cmdshape-capture-output-*")
	if err != nil {
		return nil, err
	}
	artifacts := &stagedCaptureArtifacts{dir: dir, files: make(map[string]*os.File, 4)}
	for _, name := range []string{
		replay.OutputFileName,
		replay.OutputStdoutFileName,
		replay.OutputStderrFileName,
		replay.DecisionsFileName,
	} {
		file, openErr := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
		if openErr != nil {
			artifacts.cleanup()
			return nil, openErr
		}
		artifacts.files[name] = file
	}
	return artifacts, nil
}

func (a *stagedCaptureArtifacts) writers() core.ReplayWriters {
	return core.ReplayWriters{
		Output:    a.files[replay.OutputFileName],
		Stdout:    a.files[replay.OutputStdoutFileName],
		Stderr:    a.files[replay.OutputStderrFileName],
		Decisions: a.files[replay.DecisionsFileName],
	}
}

func (a *stagedCaptureArtifacts) closeAndSync() error {
	var err error
	for _, name := range []string{
		replay.OutputFileName,
		replay.OutputStdoutFileName,
		replay.OutputStderrFileName,
		replay.DecisionsFileName,
	} {
		file := a.files[name]
		if file == nil {
			continue
		}
		err = errors.Join(err, file.Sync(), file.Close())
		a.files[name] = nil
	}
	return err
}

func (a *stagedCaptureArtifacts) promote(destinations map[string]string) error {
	for _, name := range []string{
		replay.OutputFileName,
		replay.OutputStdoutFileName,
		replay.OutputStderrFileName,
		replay.DecisionsFileName,
	} {
		if err := copyCaptureArtifact(filepath.Join(a.dir, name), destinations[name]); err != nil {
			return err
		}
	}
	return nil
}

func (a *stagedCaptureArtifacts) cleanup() {
	for name, file := range a.files {
		if file != nil {
			_ = file.Close()
			a.files[name] = nil
		}
	}
	_ = os.RemoveAll(a.dir)
}

func copyCaptureArtifact(sourcePath, destinationPath string) (err error) {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer closeWithErr(source, &err)
	destination, err := os.CreateTemp(filepath.Dir(destinationPath), "."+filepath.Base(destinationPath)+".tmp-*")
	if err != nil {
		return err
	}
	destinationPathTmp := destination.Name()
	replaced := false
	defer func() {
		if destination != nil {
			err = errors.Join(err, destination.Close())
		}
		if !replaced {
			removeErr := os.Remove(destinationPathTmp)
			if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				err = errors.Join(err, removeErr)
			}
		}
	}()
	if _, err := io.Copy(destination, source); err != nil {
		return err
	}
	if err := destination.Sync(); err != nil {
		return err
	}
	if err := destination.Close(); err != nil {
		destination = nil
		return err
	}
	destination = nil
	if err := replaceCaptureArtifact(destinationPathTmp, destinationPath); err != nil {
		return err
	}
	replaced = true
	return syncCaptureDirectory(filepath.Dir(destinationPath))
}

type captureExitError struct{ code int }

func (e captureExitError) Error() string {
	return fmt.Sprintf("captured command exited with code %d", e.code)
}
func (e captureExitError) ExitCode() int { return e.code }

type stagedCapture struct {
	dir                      string
	stdoutPath, stderrPath   string
	stdoutBytes, stderrBytes int
}

func (c *stagedCapture) cleanup() { _ = os.RemoveAll(c.dir) }

func runNativeCapture(args []string) ([]replay.Event, int, error) {
	ctx, stop := core.DefaultExecutionContext(context.Background())
	defer stop()
	return runNativeCaptureContext(ctx, args)
}

func runNativeCaptureContext(ctx context.Context, args []string) ([]replay.Event, int, error) {
	recording, exitCode, err := runNativeCaptureStagedContext(ctx, args)
	if err != nil {
		return nil, 0, err
	}
	defer recording.cleanup()
	events, err := replay.ReadEvents(recording.stdoutPath, recording.stderrPath)
	return events, exitCode, err
}

func runNativeCaptureStaged(args []string) (*stagedCapture, int, error) {
	ctx, stop := core.DefaultExecutionContext(context.Background())
	defer stop()
	return runNativeCaptureStagedContext(ctx, args)
}

func runNativeCaptureStagedContext(ctx context.Context, args []string) (_ *stagedCapture, exitCode int, retErr error) {
	dir, err := os.MkdirTemp("", "cmdshape-capture-*")
	if err != nil {
		return nil, 0, err
	}
	recording := &stagedCapture{
		dir: dir, stdoutPath: filepath.Join(dir, replay.StdoutFileName), stderrPath: filepath.Join(dir, replay.StderrFileName),
	}
	defer func() {
		if retErr != nil {
			recording.cleanup()
		}
	}()
	stdoutFile, err := os.OpenFile(recording.stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return nil, 0, err
	}
	stderrFile, err := os.OpenFile(recording.stderrPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		_ = stdoutFile.Close()
		return nil, 0, err
	}
	stdoutWriter := bufio.NewWriter(stdoutFile)
	stderrWriter := bufio.NewWriter(stderrFile)

	cmd, stdout, stderr, err := core.CommandWithPipesContext(ctx, args[0], args[1:])
	if err != nil {
		_ = stdoutFile.Close()
		_ = stderrFile.Close()
		return nil, 0, err
	}

	var (
		wg                             sync.WaitGroup
		readErrs                       = make(chan error, 2)
		sequence                       atomic.Int64
		stdoutStageErr, stderrStageErr error
	)

	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		_ = stdoutFile.Close()
		_ = stderrFile.Close()
		return nil, 0, err
	}

	wg.Go(func() {
		readErrs <- readSequencedCapture(stdout, contracts.StreamStdout, &sequence, func(seq int, stream contracts.Stream, line string) {
			recording.stdoutBytes += len(line)
			if stdoutStageErr == nil {
				stdoutStageErr = replay.WriteSequencedEvent(stdoutWriter, replay.Event{Sequence: seq, Stream: stream, Line: line})
			}
		})
	})
	wg.Go(func() {
		readErrs <- readSequencedCapture(stderr, contracts.StreamStderr, &sequence, func(seq int, stream contracts.Stream, line string) {
			recording.stderrBytes += len(line)
			if stderrStageErr == nil {
				stderrStageErr = replay.WriteSequencedEvent(stderrWriter, replay.Event{Sequence: seq, Stream: stream, Line: line})
			}
		})
	})
	wg.Wait()
	close(readErrs)
	waitErr := cmd.Wait()
	if waitErr != nil && cmd.ProcessState != nil && cmd.ProcessState.Success() {
		waitErr = nil
	}
	readErr := errors.Join(stdoutStageErr, stderrStageErr, stdoutWriter.Flush(), stderrWriter.Flush(), stdoutFile.Sync(), stderrFile.Sync(), stdoutFile.Close(), stderrFile.Close())
	for err := range readErrs {
		readErr = errors.Join(readErr, err)
	}
	return finishStagedCapture(ctx, recording, waitErr, readErr)
}

func finishStagedCapture(ctx context.Context, recording *stagedCapture, waitErr, readErr error) (*stagedCapture, int, error) {
	if readErr != nil {
		return nil, 0, errors.Join(readErr, waitErr)
	}
	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](waitErr); ok {
			exitCode = captureExitCode(exitErr)
		} else {
			return nil, 0, waitErr
		}
	}
	if signalCode, ok := core.ForwardedSignalExitCode(ctx, exitCode); ok {
		return recording, signalCode, nil
	}
	if ctx.Err() != nil {
		return nil, 0, errors.Join(ctx.Err(), waitErr)
	}
	return recording, exitCode, nil
}

func readSequencedCapture(src io.ReadCloser, stream contracts.Stream, sequence *atomic.Int64, record func(int, contracts.Stream, string)) error {
	defer func() { _ = src.Close() }()
	reader := bufio.NewReader(src)
	for {
		first, err := reader.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("read %s stream: %w", stream, err)
		}
		currentSeq := int(sequence.Add(1) - 1)
		currentRecord, readErr := replay.CompleteStreamRecord(reader, first)
		record(currentSeq, stream, string(currentRecord))
		if readErr != nil {
			if errors.Is(readErr, replay.ErrStreamRecordLimit) {
				continue
			}
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return fmt.Errorf("read %s stream: %w", stream, readErr)
		}
	}
}

func streamBytes(events []replay.Event, stream contracts.Stream) int {
	total := 0
	for _, event := range events {
		if event.Stream == stream {
			total += len(event.Line)
		}
	}
	return total
}
