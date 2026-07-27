package replay

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/SuppieRK/cmdshape/internal/contracts"
	"github.com/SuppieRK/cmdshape/internal/projectfiles"

	"gopkg.in/yaml.v3"
)

const (
	CommandFileName         = "command.yaml"
	StdoutFileName          = "stdout.txt"
	StderrFileName          = "stderr.txt"
	OutputFileName          = "output.txt"
	OutputStdoutFileName    = "output.stdout.txt"
	OutputStderrFileName    = "output.stderr.txt"
	DecisionsFileName       = "decisions.txt"
	DispatchFileName        = "dispatch.txt"
	VerifyOutputFileName    = "verify-output.txt"
	VerifyStdoutFileName    = "verify-stdout.txt"
	VerifyStderrFileName    = "verify-stderr.txt"
	VerifyDecisionsFileName = "verify-decisions.txt"
	VerifyDispatchFileName  = "verify-dispatch.txt"
	encodedPayloadPrefix    = "@cmdshape/base64:"
)

type CommandSpec struct {
	Argv             []string `yaml:"argv"`
	ExitCode         int      `yaml:"exit_code,omitempty"`
	Redacted         bool     `yaml:"redacted,omitempty"`
	ExitCodeAsserted bool     `yaml:"-"`
}

type Event struct {
	Sequence int
	Stream   contracts.Stream
	Line     string
}

type Fixture struct {
	Dir              string
	Command          CommandSpec
	CommandPath      string
	StdoutPath       string
	StderrPath       string
	OutputPath       string
	OutputStdoutPath string
	OutputStderrPath string
	DecisionsPath    string
	DispatchPath     string
	VerifyOutput     string
	VerifyStdout     string
	VerifyStderr     string
	VerifyDecisions  string
	VerifyDispatch   string
}

func FixturePaths(dir string) map[string]string {
	return map[string]string{
		CommandFileName:         filepath.Join(dir, CommandFileName),
		StdoutFileName:          filepath.Join(dir, StdoutFileName),
		StderrFileName:          filepath.Join(dir, StderrFileName),
		OutputFileName:          filepath.Join(dir, OutputFileName),
		OutputStdoutFileName:    filepath.Join(dir, OutputStdoutFileName),
		OutputStderrFileName:    filepath.Join(dir, OutputStderrFileName),
		DecisionsFileName:       filepath.Join(dir, DecisionsFileName),
		DispatchFileName:        filepath.Join(dir, DispatchFileName),
		VerifyOutputFileName:    filepath.Join(dir, VerifyOutputFileName),
		VerifyStdoutFileName:    filepath.Join(dir, VerifyStdoutFileName),
		VerifyStderrFileName:    filepath.Join(dir, VerifyStderrFileName),
		VerifyDecisionsFileName: filepath.Join(dir, VerifyDecisionsFileName),
		VerifyDispatchFileName:  filepath.Join(dir, VerifyDispatchFileName),
	}
}

func WriteCommandWithExitCode(path string, args []string, exitCode int) error {
	return WriteCommandWithExitCodeMode(path, args, exitCode, false, 0o644)
}

func WriteCommandWithExitCodeMode(path string, args []string, exitCode int, redacted bool, mode os.FileMode) error {
	root := yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "argv"},
			{
				Kind:  yaml.SequenceNode,
				Style: yaml.FlowStyle,
			},
		},
	}
	for _, arg := range args {
		root.Content[1].Content = append(root.Content[1].Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Style: yaml.DoubleQuotedStyle,
			Value: arg,
		})
	}
	root.Content = append(root.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "exit_code"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(exitCode)},
	)
	if redacted {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "redacted"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"},
		)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return fmt.Errorf("encode command yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("close command yaml encoder: %w", err)
	}
	return WriteArtifact(path, buf.Bytes(), mode)
}

func ReadCommand(path string) (CommandSpec, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return CommandSpec{}, fmt.Errorf("read command fixture: %w", err)
	}
	var spec CommandSpec
	if err := yaml.Unmarshal(body, &spec); err != nil {
		return CommandSpec{}, fmt.Errorf("parse command fixture: %w", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		return CommandSpec{}, fmt.Errorf("parse command fixture: %w", err)
	}
	spec.ExitCodeAsserted = mappingHasKey(&document, "exit_code")
	if len(spec.Argv) == 0 {
		return CommandSpec{}, fmt.Errorf("parse command fixture: argv is required")
	}
	for idx, arg := range spec.Argv {
		if strings.TrimSpace(arg) == "" {
			return CommandSpec{}, fmt.Errorf("parse command fixture: argv[%d] must be non-empty", idx)
		}
	}
	return spec, nil
}

func WriteSequenced(path string, events []Event) error {
	return WriteSequencedMode(path, events, 0o644)
}

func WriteSequencedMode(path string, events []Event, mode os.FileMode) error {
	sorted := slices.Clone(events)
	slices.SortFunc(sorted, func(a, b Event) int {
		return a.Sequence - b.Sequence
	})
	var buf strings.Builder
	for _, event := range sorted {
		_, _ = fmt.Fprintf(&buf, "%05d|", event.Sequence)
		if replayPayloadNeedsEncoding(event.Line) {
			buf.WriteString(encodedPayloadPrefix)
			buf.WriteString(base64.StdEncoding.EncodeToString([]byte(event.Line)))
			buf.WriteByte('\n')
			continue
		}
		buf.WriteString(event.Line)
	}
	return WriteArtifact(path, []byte(buf.String()), mode)
}

func WriteArtifact(path string, body []byte, perm os.FileMode) error {
	if err := projectfiles.RejectSymlinkPath(path); err != nil {
		return err
	}
	return os.WriteFile(path, body, perm)
}

func WriteSequencedEvents(path string, events []Event, stream contracts.Stream) error {
	return WriteSequencedEventsMode(path, events, stream, 0o644)
}

func WriteSequencedEventsMode(path string, events []Event, stream contracts.Stream, mode os.FileMode) error {
	selected := make([]Event, 0, len(events))
	for _, event := range events {
		if event.Stream == stream {
			selected = append(selected, event)
		}
	}
	return WriteSequencedMode(path, selected, mode)
}

func ReadSequenced(path string, stream contracts.Stream) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open sequenced stream: %w", err)
	}
	defer func() { _ = f.Close() }()
	return readSequencedFromReader(f, stream, path)
}

func ReadSequencedReader(r io.Reader, stream contracts.Stream) ([]Event, error) {
	if r == nil {
		return nil, nil
	}
	return readSequencedFromReader(r, stream, string(stream))
}

func ReadEvents(stdoutPath, stderrPath string) ([]Event, error) {
	stdoutEvents, err := ReadSequenced(stdoutPath, contracts.StreamStdout)
	if err != nil {
		return nil, err
	}
	stderrEvents, err := ReadSequenced(stderrPath, contracts.StreamStderr)
	if err != nil {
		return nil, err
	}
	return MergeAndValidate(stdoutEvents, stderrEvents)
}

func ReadEventReaders(stdout, stderr io.Reader) ([]Event, error) {
	stdoutEvents, err := ReadSequencedReader(stdout, contracts.StreamStdout)
	if err != nil {
		return nil, err
	}
	stderrEvents, err := ReadSequencedReader(stderr, contracts.StreamStderr)
	if err != nil {
		return nil, err
	}
	return MergeAndValidate(stdoutEvents, stderrEvents)
}

func readSequencedFromReader(r io.Reader, stream contracts.Stream, path string) ([]Event, error) {
	reader := bufio.NewReader(r)
	events := make([]Event, 0, 32)
	for {
		line, err := readReplayLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return events, nil
			}
			return nil, fmt.Errorf("read sequenced stream %s: %w", path, err)
		}
		sep := strings.IndexByte(line, '|')
		if sep <= 0 {
			return nil, fmt.Errorf("read sequenced stream %s: invalid prefix %q", path, line)
		}
		sequence, err := strconv.Atoi(line[:sep])
		if err != nil {
			return nil, fmt.Errorf("read sequenced stream %s: invalid sequence %q: %w", path, line[:sep], err)
		}
		payload, decodeErr := decodeReplayPayload(line[sep+1:])
		if decodeErr != nil {
			return nil, fmt.Errorf("read sequenced stream %s: %w", path, decodeErr)
		}
		events = append(events, Event{
			Sequence: sequence,
			Stream:   stream,
			Line:     payload,
		})
	}
}

func MergeAndValidate(stdout, stderr []Event) ([]Event, error) {
	events := make([]Event, 0, len(stdout)+len(stderr))
	events = append(events, stdout...)
	events = append(events, stderr...)
	slices.SortFunc(events, func(a, b Event) int {
		return a.Sequence - b.Sequence
	})
	if err := ValidateSequence(events); err != nil {
		return nil, err
	}
	return events, nil
}

func ValidateSequence(events []Event) error {
	for idx, event := range events {
		if event.Sequence != idx {
			return fmt.Errorf("replay sequence break: expected %05d, got %05d", idx, event.Sequence)
		}
	}
	return nil
}

func CombinedInput(events []Event) string {
	var out strings.Builder
	for _, event := range events {
		out.WriteString(event.Line)
	}
	return out.String()
}

func HasRequiredFixtureFiles(dir string) bool {
	paths := FixturePaths(dir)
	return slices.ContainsFunc([]string{StdoutFileName, StderrFileName, OutputFileName}, func(name string) bool {
		if info, err := os.Stat(paths[name]); err == nil && !info.IsDir() {
			return true
		}
		return false
	})
}

func LoadFixture(dir string) (Fixture, error) {
	resolved, err := filepath.Abs(dir)
	if err != nil {
		return Fixture{}, fmt.Errorf("resolve fixture directory: %w", err)
	}
	paths := FixturePaths(resolved)
	command, err := ReadCommand(paths[CommandFileName])
	if err != nil {
		return Fixture{}, err
	}
	if !HasRequiredFixtureFiles(resolved) {
		return Fixture{}, fmt.Errorf("fixture %q must contain at least one of %s, %s, or %s", resolved, StdoutFileName, StderrFileName, OutputFileName)
	}
	return Fixture{
		Dir:              resolved,
		Command:          command,
		CommandPath:      paths[CommandFileName],
		StdoutPath:       paths[StdoutFileName],
		StderrPath:       paths[StderrFileName],
		OutputPath:       paths[OutputFileName],
		OutputStdoutPath: paths[OutputStdoutFileName],
		OutputStderrPath: paths[OutputStderrFileName],
		DecisionsPath:    paths[DecisionsFileName],
		DispatchPath:     paths[DispatchFileName],
		VerifyOutput:     paths[VerifyOutputFileName],
		VerifyStdout:     paths[VerifyStdoutFileName],
		VerifyStderr:     paths[VerifyStderrFileName],
		VerifyDecisions:  paths[VerifyDecisionsFileName],
		VerifyDispatch:   paths[VerifyDispatchFileName],
	}, nil
}

func readReplayLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if len(line) > 0 {
		return line, nil
	}
	return "", err
}

// ReadStreamRecord reads one live-output record. Bare carriage returns are
// record boundaries, while CRLF remains a single boundary.
func ReadStreamRecord(reader *bufio.Reader) ([]byte, error) {
	first, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	return CompleteStreamRecord(reader, first)
}

// CompleteStreamRecord finishes a record after its first byte has already
// been observed. Capture uses this form so cross-stream sequence numbers are
// assigned at the same point that native output first becomes visible.
func CompleteStreamRecord(reader *bufio.Reader, first byte) ([]byte, error) {
	record := make([]byte, 0, 256)
	record = append(record, first)
	for {
		switch record[len(record)-1] {
		case '\n':
			return record, nil
		case '\r':
			if next, peekErr := reader.Peek(1); peekErr == nil && next[0] == '\n' {
				newline, readErr := reader.ReadByte()
				if readErr != nil {
					return record, readErr
				}
				record = append(record, newline)
			}
			return record, nil
		}

		next, err := reader.ReadByte()
		if err != nil {
			return record, err
		}
		record = append(record, next)
	}
}

func replayPayloadNeedsEncoding(line string) bool {
	if !strings.HasSuffix(line, "\n") || strings.Count(line, "\n") != 1 {
		return true
	}
	return strings.HasPrefix(line, encodedPayloadPrefix)
}

func decodeReplayPayload(payload string) (string, error) {
	encoded, ok := strings.CutPrefix(payload, encodedPayloadPrefix)
	if !ok {
		return payload, nil
	}
	encoded = strings.TrimSuffix(encoded, "\n")
	body, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode base64 replay payload: %w", err)
	}
	return string(body), nil
}

func mappingHasKey(document *yaml.Node, key string) bool {
	if document == nil || len(document.Content) == 0 {
		return false
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return false
	}
	for index := 0; index+1 < len(root.Content); index += 2 {
		if root.Content[index].Value == key {
			return true
		}
	}
	return false
}
