package replay

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"go-command-compression-proxy/internal/contracts"

	"gopkg.in/yaml.v3"
)

const (
	CommandFileName         = "command.yaml"
	StdoutFileName          = "stdout.txt"
	StderrFileName          = "stderr.txt"
	OutputFileName          = "output.txt"
	DecisionsFileName       = "decisions.txt"
	VerifyOutputFileName    = "verify-output.txt"
	VerifyDecisionsFileName = "verify-decisions.txt"
)

type CommandSpec struct {
	Argv []string `yaml:"argv"`
}

type Event struct {
	Sequence int
	Stream   contracts.Stream
	Line     string
}

type Fixture struct {
	Dir             string
	Command         CommandSpec
	CommandPath     string
	StdoutPath      string
	StderrPath      string
	OutputPath      string
	DecisionsPath   string
	VerifyOutput    string
	VerifyDecisions string
}

func FixturePaths(dir string) map[string]string {
	return map[string]string{
		CommandFileName:         filepath.Join(dir, CommandFileName),
		StdoutFileName:          filepath.Join(dir, StdoutFileName),
		StderrFileName:          filepath.Join(dir, StderrFileName),
		OutputFileName:          filepath.Join(dir, OutputFileName),
		DecisionsFileName:       filepath.Join(dir, DecisionsFileName),
		VerifyOutputFileName:    filepath.Join(dir, VerifyOutputFileName),
		VerifyDecisionsFileName: filepath.Join(dir, VerifyDecisionsFileName),
	}
}

func WriteCommand(path string, args []string) error {
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
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return fmt.Errorf("encode command yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("close command yaml encoder: %w", err)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func WriteCommandFile(path string, args []string) error {
	return WriteCommand(path, args)
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
	sorted := slices.Clone(events)
	slices.SortFunc(sorted, func(a, b Event) int {
		return a.Sequence - b.Sequence
	})
	var buf strings.Builder
	for _, event := range sorted {
		_, _ = fmt.Fprintf(&buf, "%05d|%s", event.Sequence, event.Line)
		if !strings.HasSuffix(event.Line, "\n") {
			buf.WriteByte('\n')
		}
	}
	return os.WriteFile(path, []byte(buf.String()), 0o644)
}

func WriteSequencedEvents(path string, events []Event, stream contracts.Stream) error {
	selected := make([]Event, 0, len(events))
	for _, event := range events {
		if event.Stream == stream {
			selected = append(selected, event)
		}
	}
	return WriteSequenced(path, selected)
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

func readSequencedFromReader(r io.Reader, stream contracts.Stream, path string) ([]Event, error) {
	reader := bufio.NewReader(r)
	events := make([]Event, 0, 32)
	for {
		line, err := readReplayLine(reader)
		if err != nil {
			if err == io.EOF {
				return events, nil
			}
			return nil, fmt.Errorf("read sequenced stream %s: %w", path, err)
		}
		if len(line) < 6 || line[5] != '|' {
			return nil, fmt.Errorf("read sequenced stream %s: invalid prefix %q", path, line)
		}
		sequence, err := strconv.Atoi(line[:5])
		if err != nil {
			return nil, fmt.Errorf("read sequenced stream %s: invalid sequence %q: %w", path, line[:5], err)
		}
		events = append(events, Event{
			Sequence: sequence,
			Stream:   stream,
			Line:     line[6:],
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
	for _, name := range []string{StdoutFileName, StderrFileName, OutputFileName} {
		if info, err := os.Stat(paths[name]); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
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
		Dir:             resolved,
		Command:         command,
		CommandPath:     paths[CommandFileName],
		StdoutPath:      paths[StdoutFileName],
		StderrPath:      paths[StderrFileName],
		OutputPath:      paths[OutputFileName],
		DecisionsPath:   paths[DecisionsFileName],
		VerifyOutput:    paths[VerifyOutputFileName],
		VerifyDecisions: paths[VerifyDecisionsFileName],
	}, nil
}

func readReplayLine(reader *bufio.Reader) (string, error) {
	var current []byte
	pendingCR := false
	for {
		b, err := reader.ReadByte()
		if err == io.EOF {
			return finishReplayLine(current, pendingCR)
		}
		if err != nil {
			return "", err
		}
		line, done, nextPendingCR := appendReplayLineByte(current, b, pendingCR)
		if done {
			return line, nil
		}
		current = append(current, b)
		pendingCR = nextPendingCR
	}
}

func appendReplayLineByte(current []byte, b byte, pendingCR bool) (string, bool, bool) {
	if pendingCR && b == '\n' {
		return string(append(current, '\n')), true, false
	}

	switch b {
	case '\r':
		return "", false, true
	case '\n':
		return string(append(current, '\n')), true, false
	default:
		return "", false, false
	}
}

func finishReplayLine(current []byte, pendingCR bool) (string, error) {
	if pendingCR && len(current) > 0 {
		current = current[:len(current)-1]
	}
	if len(current) == 0 {
		return "", io.EOF
	}
	return string(current), nil
}
