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
	captureStdoutFileName = "stdout.txt"
	captureStderrFileName = "stderr.txt"
	captureOutputFileName = "output.txt"
)

type captureVerifier interface {
	ReplayWithExitCode(args []string, events []replay.Event, exitCode int) (core.ReplayResult, error)
}

var newCaptureRunner = func() captureVerifier {
	return core.NewRunnerWithOptions(core.Options{})
}

func RunCapture(args []string) error {
	recordFailure := func(commandArgs []string, dir, stage string, err error) error {
		audit.MustAppend("capture_invocation_finish", map[string]any{
			"command": strings.Join(commandArgs, " "),
			"dir":     dir,
			"success": false,
			"stage":   stage,
			"error":   err.Error(),
		})
		return err
	}

	fs := newLifecycleFlagSet("capture")
	dirFlag := fs.String("dir", "", "directory where capture artifacts are written")
	setLifecycleUsage(
		fs,
		"capture native stdout/stderr and replay CCP output for local filter iteration",
		[]string{"ccp capture [--dir <path>] -- <command> [args...]"},
		"capture writes command.yaml, stdout.txt, stderr.txt, and output.txt.",
		"when --dir is omitted, capture writes to the current working directory.",
		"stdout.txt and stderr.txt use sequenced 00000| prefixes so replay preserves cross-stream ordering.",
		"capture runs the command natively once, then replays the captured streams through the current YAML runtime.",
		"non-zero command exits still write artifacts so the failure can be iterated on locally.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return recordFailure(nil, "", "parse_flags", err)
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
		return recordFailure(commandArgs, dirValue, "validate_flags", fmt.Errorf("missing command after '--'"))
	}

	captureDir, err := resolveCaptureDir(dirValue, commandArgs[0])
	if err != nil {
		return recordFailure(commandArgs, dirValue, "resolve_dir", err)
	}
	audit.MustAppend("capture_invocation_start", map[string]any{
		"command": strings.Join(commandArgs, " "),
		"dir":     captureDir,
	})

	events, exitCode, err := runNativeCapture(commandArgs)
	if err != nil {
		return recordFailure(commandArgs, captureDir, "native_exec", err)
	}
	if err := os.MkdirAll(captureDir, 0o755); err != nil {
		return recordFailure(commandArgs, captureDir, "mkdir", err)
	}

	commandPath := filepath.Join(captureDir, replay.CommandFileName)
	stdoutPath := filepath.Join(captureDir, captureStdoutFileName)
	stderrPath := filepath.Join(captureDir, captureStderrFileName)
	outputPath := filepath.Join(captureDir, captureOutputFileName)
	if err := replay.WriteCommandWithExitCode(commandPath, commandArgs, exitCode); err != nil {
		return recordFailure(commandArgs, captureDir, "write_command", err)
	}
	if err := replay.WriteSequencedEvents(stdoutPath, events, contracts.StreamStdout); err != nil {
		return recordFailure(commandArgs, captureDir, "write_stdout", err)
	}
	if err := replay.WriteSequencedEvents(stderrPath, events, contracts.StreamStderr); err != nil {
		return recordFailure(commandArgs, captureDir, "write_stderr", err)
	}

	replayed, err := newCaptureRunner().ReplayWithExitCode(commandArgs, events, exitCode)
	if err != nil {
		return recordFailure(commandArgs, captureDir, "replay_output", err)
	}
	if err := replay.WriteArtifact(outputPath, []byte(replayed.Output), 0o644); err != nil {
		return recordFailure(commandArgs, captureDir, "write_output", err)
	}

	audit.MustAppend("capture_invocation_finish", map[string]any{
		"command":      strings.Join(commandArgs, " "),
		"dir":          captureDir,
		"command_path": commandPath,
		"stdout_path":  stdoutPath,
		"stderr_path":  stderrPath,
		"output_path":  outputPath,
		"exit_code":    exitCode,
		"stdout_bytes": streamBytes(events, contracts.StreamStdout),
		"stderr_bytes": streamBytes(events, contracts.StreamStderr),
		"output_bytes": len(replayed.Output),
		"success":      true,
	})

	if exitCode != 0 {
		return fmt.Errorf("captured command exited with code %d", exitCode)
	}
	return nil
}

func resolveCaptureDir(dir, _ string) (string, error) {
	if strings.TrimSpace(dir) != "" {
		return filepath.Abs(dir)
	}
	return os.Getwd()
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
	var (
		currentLine []byte
		currentSeq  = -1
	)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			finishSequencedCaptureLine(&currentLine, &currentSeq, stream, record)
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("read %s stream: %w", stream, err)
		}
		appendSequencedCaptureByte(&currentLine, &currentSeq, b, stream, sequence, record)
	}
}

func appendSequencedCaptureByte(currentLine *[]byte, currentSeq *int, b byte, stream contracts.Stream, sequence *atomic.Int64, record func(int, contracts.Stream, string)) {
	ensureSequencedCaptureLine(currentLine, currentSeq, sequence)
	switch b {
	case '\n':
		emitSequencedCaptureLine(currentLine, currentSeq, true, stream, record)
	default:
		*currentLine = append(*currentLine, b)
	}
}

func ensureSequencedCaptureLine(currentLine *[]byte, currentSeq *int, sequence *atomic.Int64) {
	if len(*currentLine) > 0 || *currentSeq >= 0 {
		return
	}
	*currentSeq = int(sequence.Add(1) - 1)
}

func finishSequencedCaptureLine(currentLine *[]byte, currentSeq *int, stream contracts.Stream, record func(int, contracts.Stream, string)) {
	if len(*currentLine) == 0 {
		return
	}
	record(*currentSeq, stream, string(*currentLine))
}

func emitSequencedCaptureLine(currentLine *[]byte, currentSeq *int, includeNewline bool, stream contracts.Stream, record func(int, contracts.Stream, string)) {
	if includeNewline {
		*currentLine = append(*currentLine, '\n')
	}
	record(*currentSeq, stream, string(*currentLine))
	*currentLine = (*currentLine)[:0]
	*currentSeq = -1
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
