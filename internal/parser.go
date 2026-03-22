package core

import (
	"errors"
	"fmt"
	"strings"

	"go-command-compression-proxy/internal/contracts"
)

var errNoCommandProvided = errors.New("no command provided")

var wrapperToolVocabulary = map[string]string{
	"gradlew":   "gradle",
	"mvnw":      "mvn",
	"mvnwdebug": "mvn",
}

func ParseCommandArgs(args []string) (contracts.Command, error) {
	if len(args) == 0 {
		return contracts.Command{}, errNoCommandProvided
	}

	name := strings.TrimSpace(args[0])
	if name == "" {
		return contracts.Command{}, errNoCommandProvided
	}

	cloned := append([]string(nil), args...)
	return contracts.Command{
		RawInput: renderCommandArgs(cloned),
		Args:     cloned,
		Tool:     canonicalToolName(name),
	}, nil
}

func renderCommandArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	rendered := make([]string, len(args))
	for i, arg := range args {
		rendered[i] = renderCommandArg(arg)
	}
	return strings.Join(rendered, " ")
}

func renderCommandArg(arg string) string {
	if arg == "" {
		return `''`
	}
	if isShellSafeArg(arg) {
		return arg
	}
	return quoteShellArg(arg)
}

func isShellSafeArg(arg string) bool {
	const safe = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@%_+=:,./-"
	for _, r := range arg {
		if !strings.ContainsRune(safe, r) {
			return false
		}
	}
	return true
}

func quoteShellArg(arg string) string {
	if !strings.Contains(arg, "'") {
		return "'" + arg + "'"
	}
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range arg {
		if r == '\'' {
			b.WriteString(`'"'"'`)
			continue
		}
		b.WriteRune(r)
	}
	b.WriteByte('\'')
	return b.String()
}

func ParseCommandLine(raw string) (contracts.Command, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return contracts.Command{}, errNoCommandProvided
	}

	args, err := splitCommandLine(trimmed)
	if err != nil {
		return contracts.Command{}, err
	}

	command, err := ParseCommandArgs(args)
	if err != nil {
		return contracts.Command{}, err
	}
	command.RawInput = trimmed
	return command, nil
}

func splitCommandLine(raw string) ([]string, error) {
	state := newCommandLineSplitState()
	for _, r := range raw {
		state.consume(r)
	}

	if err := state.finish(raw); err != nil {
		return nil, err
	}
	if len(state.args) == 0 {
		return nil, errNoCommandProvided
	}
	return state.args, nil
}

func unicodeIsSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func canonicalToolName(name string) string {
	base := trimPathToBase(name)
	lower := strings.ToLower(base)

	if mapped, ok := wrapperToolVocabulary[lower]; ok {
		return mapped
	}
	if stem := trimExecutableSuffix(base); stem != base {
		if mapped, ok := wrapperToolVocabulary[strings.ToLower(stem)]; ok {
			return mapped
		}
		return stem
	}
	return base
}

func trimPathToBase(name string) string {
	idx := strings.LastIndexAny(name, `/\`)
	if idx < 0 {
		return name
	}
	return name[idx+1:]
}

func trimExecutableSuffix(name string) string {
	lower := strings.ToLower(name)
	for _, suffix := range []string{".exe", ".cmd", ".bat", ".ps1"} {
		if strings.HasSuffix(lower, suffix) {
			return name[:len(name)-len(suffix)]
		}
	}
	return name
}

type commandLineSplitState struct {
	args         []string
	current      strings.Builder
	inSingle     bool
	inDouble     bool
	escaping     bool
	escapeDouble bool
	tokenStarted bool
}

func newCommandLineSplitState() *commandLineSplitState {
	return &commandLineSplitState{}
}

func (s *commandLineSplitState) consume(r rune) {
	switch {
	case s.escaping:
		s.consumeEscaped(r)
		s.escaping = false
		s.escapeDouble = false
	case s.inSingle:
		s.consumeSingleQuoted(r)
	case s.inDouble:
		s.consumeDoubleQuoted(r)
	default:
		s.consumeUnquoted(r)
	}
}

func (s *commandLineSplitState) consumeEscaped(r rune) {
	if s.escapeDouble {
		if isDoubleQuotedEscape(r) {
			if r != '\n' {
				s.writeRune(r)
			}
			return
		}
		s.writeRune('\\')
		s.writeRune(r)
		return
	}
	if unicodeIsSpace(r) || r == '\\' || r == '\'' || r == '"' || r == ';' {
		s.writeRune(r)
		return
	}
	s.writeRune('\\')
	s.writeRune(r)
}

func isDoubleQuotedEscape(r rune) bool {
	switch r {
	case '"', '\\', '$', '`', '\n':
		return true
	default:
		return false
	}
}

func (s *commandLineSplitState) consumeSingleQuoted(r rune) {
	if r == '\'' {
		s.inSingle = false
		return
	}
	s.writeRune(r)
}

func (s *commandLineSplitState) consumeDoubleQuoted(r rune) {
	switch r {
	case '"':
		s.inDouble = false
	case '\\':
		s.escaping = true
		s.escapeDouble = true
	default:
		s.writeRune(r)
	}
}

func (s *commandLineSplitState) consumeUnquoted(r rune) {
	switch {
	case unicodeIsSpace(r):
		s.flushToken()
	case r == '\'':
		s.inSingle = true
		s.tokenStarted = true
	case r == '"':
		s.inDouble = true
		s.tokenStarted = true
	case r == '\\':
		s.escaping = true
		s.escapeDouble = false
		s.tokenStarted = true
	default:
		s.writeRune(r)
	}
}

func (s *commandLineSplitState) writeRune(r rune) {
	s.current.WriteRune(r)
	s.tokenStarted = true
}

func (s *commandLineSplitState) flushToken() {
	if !s.tokenStarted && s.current.Len() == 0 {
		return
	}
	s.args = append(s.args, s.current.String())
	s.current.Reset()
	s.tokenStarted = false
}

func (s *commandLineSplitState) finish(raw string) error {
	switch {
	case s.escaping:
		return fmt.Errorf("unterminated escape sequence in %q", raw)
	case s.inSingle:
		return fmt.Errorf("unterminated single quote in %q", raw)
	case s.inDouble:
		return fmt.Errorf("unterminated double quote in %q", raw)
	default:
		s.flushToken()
		return nil
	}
}
