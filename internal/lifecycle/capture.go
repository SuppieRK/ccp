package lifecycle

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	core "go-command-compression-proxy/internal"
	"go-command-compression-proxy/internal/audit"
	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/replay"
)

const (
	captureStdoutFileName       = "stdout.txt"
	captureStderrFileName       = "stderr.txt"
	captureOutputFileName       = "output.txt"
	captureOutputStdoutFileName = "output.stdout.txt"
	captureOutputStderrFileName = "output.stderr.txt"
)

type captureVerifier interface {
	ReplayWithExitCode(args []string, events []replay.Event, exitCode int) (core.ReplayResult, error)
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
		"capture native stdout/stderr and replay CCP output for local filter iteration",
		[]string{"ccp capture [--dir <path>] -- <command> [args...]"},
		"capture writes command.yaml, sequenced native streams, merged output.txt, and exact output.stdout.txt/output.stderr.txt expectations.",
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
		return recordCaptureFailure(commandArgs, dirValue, confidential, "validate_flags", fmt.Errorf("missing command after '--'"))
	}

	captureDir, err := resolveCaptureDir(dirValue, commandArgs[0])
	if err != nil {
		return recordCaptureFailure(commandArgs, dirValue, confidential, "resolve_dir", err)
	}
	return executeCapture(commandArgs, captureDir, confidential)
}

func recordCaptureFailure(commandArgs []string, dir string, confidential []string, stage string, err error) error {
	audit.MustAppend("capture_invocation_finish", map[string]any{
		"command": captureAuditCommand(commandArgs, confidential),
		"dir":     redactCaptureText(dir, confidential),
		"success": false,
		"stage":   stage,
		"error":   redactCaptureText(err.Error(), confidential),
	})
	return err
}

func executeCapture(commandArgs []string, captureDir string, confidential []string) error {
	auditCommand := captureAuditCommand(commandArgs, confidential)
	recordFailure := func(stage string, err error) error {
		return recordCaptureFailure(commandArgs, captureDir, confidential, stage, err)
	}
	audit.MustAppend("capture_invocation_start", map[string]any{
		"command": auditCommand,
		"dir":     redactCaptureText(captureDir, confidential),
	})

	events, exitCode, err := runNativeCapture(commandArgs)
	if err != nil {
		return recordFailure("native_exec", err)
	}
	if err := ensureCaptureDirectory(captureDir); err != nil {
		return recordFailure("mkdir", err)
	}

	commandPath := filepath.Join(captureDir, replay.CommandFileName)
	stdoutPath := filepath.Join(captureDir, captureStdoutFileName)
	stderrPath := filepath.Join(captureDir, captureStderrFileName)
	outputPath := filepath.Join(captureDir, captureOutputFileName)
	outputStdoutPath := filepath.Join(captureDir, captureOutputStdoutFileName)
	outputStderrPath := filepath.Join(captureDir, captureOutputStderrFileName)
	if err := tightenCaptureTargets([]string{commandPath, stdoutPath, stderrPath, outputPath, outputStdoutPath, outputStderrPath}); err != nil {
		return recordFailure("tighten_targets", err)
	}
	storedArgs := redactCaptureArgs(commandArgs, confidential)
	storedEvents := redactCaptureEvents(events, confidential)
	if err := replay.WriteCommandWithExitCodeMode(commandPath, storedArgs, exitCode, len(confidential) > 0, 0o600); err != nil {
		return recordFailure("write_command", err)
	}
	if err := replay.WriteSequencedEventsMode(stdoutPath, storedEvents, contracts.StreamStdout, 0o600); err != nil {
		return recordFailure("write_stdout", err)
	}
	if err := replay.WriteSequencedEventsMode(stderrPath, storedEvents, contracts.StreamStderr, 0o600); err != nil {
		return recordFailure("write_stderr", err)
	}

	replayed, err := newCaptureRunner(confidential).ReplayWithExitCode(commandArgs, events, exitCode)
	if err != nil {
		return recordFailure("replay_output", err)
	}
	if err := replay.WriteArtifact(outputPath, []byte(replayed.Output), 0o600); err != nil {
		return recordFailure("write_output", err)
	}
	if err := replay.WriteArtifact(outputStdoutPath, []byte(replayed.Stdout), 0o600); err != nil {
		return recordFailure("write_output_stdout", err)
	}
	if err := replay.WriteArtifact(outputStderrPath, []byte(replayed.Stderr), 0o600); err != nil {
		return recordFailure("write_output_stderr", err)
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
		"stdout_bytes":       streamBytes(events, contracts.StreamStdout),
		"stderr_bytes":       streamBytes(events, contracts.StreamStderr),
		"output_bytes":       len(replayed.Output),
		"success":            true,
	})

	if exitCode != 0 {
		return fmt.Errorf("captured command exited with code %d", exitCode)
	}
	return nil
}

func captureAuditCommand(commandArgs, confidential []string) string {
	return strings.Join(redactCaptureArgs(commandArgs, confidential), " ")
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

func redactCaptureEvents(events []replay.Event, confidential []string) []replay.Event {
	out := make([]replay.Event, len(events))
	for index, event := range events {
		event.Line = redactCaptureText(event.Line, confidential)
		out[index] = event
	}
	return out
}

func redactCaptureText(value string, confidential []string) string {
	for _, secret := range confidential {
		value = strings.ReplaceAll(value, secret, "***")
	}
	return value
}

func runNativeCapture(args []string) ([]replay.Event, int, error) {
	ctx, stop := core.DefaultExecutionContext(context.Background())
	defer stop()
	return runNativeCaptureContext(ctx, args)
}

func runNativeCaptureContext(ctx context.Context, args []string) ([]replay.Event, int, error) {
	cmd, stdout, stderr, err := core.CommandWithPipesContext(ctx, args[0], args[1:])
	if err != nil {
		return nil, 0, err
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		events   []replay.Event
		readErrs = make(chan error, 2)
		sequence atomic.Int64
	)
	record := func(seq int, stream contracts.Stream, line string) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, replay.Event{Sequence: seq, Stream: stream, Line: line})
	}

	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, 0, err
	}

	wg.Go(func() {
		readErrs <- readSequencedCapture(stdout, contracts.StreamStdout, &sequence, record)
	})
	wg.Go(func() {
		readErrs <- readSequencedCapture(stderr, contracts.StreamStderr, &sequence, record)
	})
	wg.Wait()
	close(readErrs)
	waitErr := cmd.Wait()
	var readErr error
	for err := range readErrs {
		readErr = errors.Join(readErr, err)
	}
	if readErr != nil {
		if waitErr != nil {
			return nil, 0, errors.Join(readErr, waitErr)
		}
		return nil, 0, readErr
	}
	if ctx.Err() != nil {
		if waitErr != nil {
			return nil, 0, errors.Join(ctx.Err(), waitErr)
		}
		return nil, 0, ctx.Err()
	}
	sort.Slice(events, func(i, j int) bool { return events[i].Sequence < events[j].Sequence })
	if err := replay.ValidateSequence(events); err != nil {
		return nil, 0, err
	}
	if waitErr != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](waitErr); ok {
			return events, exitErr.ExitCode(), nil
		}
		return nil, 0, waitErr
	}
	return events, 0, nil
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
