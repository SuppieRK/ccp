package lifecycle

import (
	"bufio"
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
	Replay(args []string, events []replay.Event) (core.ReplayResult, error)
}

var newCaptureRunner = func() captureVerifier {
	return core.NewRunner()
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
	if err := replay.WriteCommandFile(commandPath, commandArgs); err != nil {
		return recordFailure(commandArgs, captureDir, "write_command", err)
	}
	if err := replay.WriteSequencedEvents(stdoutPath, events, contracts.StreamStdout); err != nil {
		return recordFailure(commandArgs, captureDir, "write_stdout", err)
	}
	if err := replay.WriteSequencedEvents(stderrPath, events, contracts.StreamStderr); err != nil {
		return recordFailure(commandArgs, captureDir, "write_stderr", err)
	}

	replayed, err := newCaptureRunner().Replay(commandArgs, events)
	if err != nil {
		return recordFailure(commandArgs, captureDir, "replay_output", err)
	}
	if err := os.WriteFile(outputPath, []byte(replayed.Output), 0o644); err != nil {
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
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, 0, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		return nil, 0, err
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		events   []replay.Event
		sequence atomic.Int64
	)
	record := func(stream contracts.Stream, line string) {
		seq := int(sequence.Add(1) - 1)
		mu.Lock()
		defer mu.Unlock()
		events = append(events, replay.Event{Sequence: seq, Stream: stream, Line: line})
	}

	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, 0, err
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		readSequencedCapture(stdout, contracts.StreamStdout, record)
	}()
	go func() {
		defer wg.Done()
		readSequencedCapture(stderr, contracts.StreamStderr, record)
	}()
	wg.Wait()
	waitErr := cmd.Wait()
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

func readSequencedCapture(src io.ReadCloser, stream contracts.Stream, record func(contracts.Stream, string)) {
	defer func() { _ = src.Close() }()
	reader := bufio.NewReader(src)
	var currentLine []byte
	pendingCR := false
	for {
		b, err := reader.ReadByte()
		if err != nil {
			finishSequencedCaptureLine(&currentLine, pendingCR, stream, record)
			return
		}
		pendingCR = appendSequencedCaptureByte(&currentLine, b, pendingCR, stream, record)
	}
}

func appendSequencedCaptureByte(currentLine *[]byte, b byte, pendingCR bool, stream contracts.Stream, record func(contracts.Stream, string)) bool {
	if pendingCR {
		if b == '\n' {
			emitSequencedCaptureLine(currentLine, true, stream, record)
			return false
		}
		*currentLine = (*currentLine)[:0]
	}

	switch b {
	case '\r':
		return true
	case '\n':
		emitSequencedCaptureLine(currentLine, true, stream, record)
	default:
		*currentLine = append(*currentLine, b)
	}
	return false
}

func finishSequencedCaptureLine(currentLine *[]byte, pendingCR bool, stream contracts.Stream, record func(contracts.Stream, string)) {
	if pendingCR {
		*currentLine = (*currentLine)[:0]
	}
	if len(*currentLine) == 0 {
		return
	}
	record(stream, string(*currentLine))
}

func emitSequencedCaptureLine(currentLine *[]byte, includeNewline bool, stream contracts.Stream, record func(contracts.Stream, string)) {
	if includeNewline {
		*currentLine = append(*currentLine, '\n')
	}
	record(stream, string(*currentLine))
	*currentLine = (*currentLine)[:0]
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
