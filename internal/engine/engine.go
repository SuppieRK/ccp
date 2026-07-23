package engine

import (
	"bytes"
	"slices"
	"sync"

	"go-command-compression-proxy/internal/contracts"
)

const CandidateBufferLimit = 8 * 1024 * 1024

type Engine struct {
	registry *Registry
}

func NewEngine(registry *Registry) *Engine {
	if registry == nil {
		panic("engine registry must not be nil")
	}
	return &Engine{registry: registry}
}

type State struct {
	filter      contracts.Filter
	command     contracts.Command
	buffer      *OrderedBuffer
	rawEntries  []BufferEntry
	recovery    []BufferEntry
	rawBytes    int
	bufferLimit int
	next        uint64
	exitCode    int
	passthrough bool
	mu          sync.Mutex
}

func newState(command contracts.Command, filter contracts.Filter) *State {
	return newStateWithLimit(command, filter, CandidateBufferLimit)
}

func newStateWithLimit(command contracts.Command, filter contracts.Filter, bufferLimit int) *State {
	return &State{
		filter:      filter,
		command:     command,
		buffer:      NewOrderedBuffer(),
		rawEntries:  make([]BufferEntry, 0, 64),
		bufferLimit: bufferLimit,
	}
}

func (e *Engine) Start(command contracts.Command) *State {
	return newState(command, e.registry.Resolve(command))
}

func (s *State) Stdout(line string) []BufferEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, entries := s.applyAction(contracts.StreamStdout, line, s.filter.OnStdout(line, s))
	return entries
}

func (s *State) Stderr(line string) []BufferEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, entries := s.applyAction(contracts.StreamStderr, line, s.filter.OnStderr(line, s))
	return entries
}

func (s *State) Exit(exitCode int) []BufferEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exitCode = exitCode
	return s.applyExitActions(s.exitActions())
}

func (s *State) StdoutAction(line string) (contracts.Action, []BufferEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyAction(contracts.StreamStdout, line, s.filter.OnStdout(line, s))
}

func (s *State) StderrAction(line string) (contracts.Action, []BufferEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyAction(contracts.StreamStderr, line, s.filter.OnStderr(line, s))
}

func (s *State) ExitAction(exitCode int) (contracts.Action, []BufferEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exitCode = exitCode
	actions := s.exitActions()
	action := contracts.Action{Kind: contracts.ActionKeep}
	if len(actions) > 0 {
		action = actions[0]
	}
	return action, s.applyExitActions(actions)
}

func (s *State) exitActions() []contracts.Action {
	if filter, ok := s.filter.(contracts.ExitActionsFilter); ok {
		return filter.OnStdoutExitActions(s)
	}
	return []contracts.Action{s.filter.OnStdoutExit(s)}
}

func (s *State) applyExitActions(actions []contracts.Action) []BufferEntry {
	if s.passthrough {
		return nil
	}
	s.recovery = slices.Clone(s.rawEntries)
	for _, action := range actions {
		target := action.Stream
		if target == "" {
			target = contracts.StreamStdout
		}
		if target == contracts.StreamCombined {
			stream, mixed := singleBufferedStream(s.buffer.Entries())
			if mixed {
				s.passthrough = true
				return s.takeRawEntries()
			}
			if stream != "" {
				target = stream
			} else {
				target = contracts.StreamStdout
			}
		}

		switch action.Kind {
		case contracts.ActionIgnore:
			s.buffer.RemoveLast(target, s.buffer.Count(target))
		case contracts.ActionReplace:
			removed := s.buffer.RemoveLastEntries(target, exitReplaceCount(s.buffer, target, action.ReplaceCount))
			if action.Output != "" {
				sequence := s.next
				original := []byte(nil)
				if len(removed) > 0 {
					sequence = removed[0].Sequence
					original = joinOriginal(removed)
				} else {
					s.next++
				}
				s.buffer.AddAt(sequence, target, original, []byte(action.Output))
			}
		}
	}
	entries := s.buffer.Entries()
	if s.rawBytes > 0 && transformedBytes(entries) >= s.rawBytes {
		s.passthrough = true
		s.buffer.Clear()
		return s.takeRawEntries()
	}
	s.rawEntries = nil
	s.buffer.Clear()
	return entries
}

func transformedBytes(entries []BufferEntry) int {
	total := 0
	for _, entry := range entries {
		total += len(entry.Transformed)
	}
	return total
}

func singleBufferedStream(entries []BufferEntry) (contracts.Stream, bool) {
	var stream contracts.Stream
	for _, entry := range entries {
		if stream == "" {
			stream = entry.Stream
			continue
		}
		if stream != entry.Stream {
			return "", true
		}
	}
	return stream, false
}

func joinOriginal(entries []BufferEntry) []byte {
	var out bytes.Buffer
	for _, entry := range entries {
		_, _ = out.Write(entry.Original)
	}
	return out.Bytes()
}

func (s *State) applyAction(stream contracts.Stream, line string, action contracts.Action) (contracts.Action, []BufferEntry) {
	sequence := s.next
	s.next++
	raw := newBufferEntry(sequence, stream, []byte(line), []byte(line))
	if s.passthrough {
		return action, []BufferEntry{raw}
	}
	s.rawEntries = append(s.rawEntries, raw)
	s.rawBytes += len(raw.Original)
	if s.bufferLimit > 0 && s.rawBytes > s.bufferLimit {
		s.passthrough = true
		s.buffer.Clear()
		return action, s.takeRawEntries()
	}

	switch action.Kind {
	case contracts.ActionIgnore:
		return action, nil
	case contracts.ActionEmit:
		s.rawEntries = s.rawEntries[:len(s.rawEntries)-1]
		s.rawBytes -= len(raw.Original)
		return action, []BufferEntry{raw}
	case contracts.ActionReplace:
		s.buffer.AddAt(sequence, stream, raw.Original, raw.Original)
		removed := s.buffer.RemoveLastEntries(stream, max(action.ReplaceCount, 1))
		if action.Output != "" {
			replacementSequence := sequence
			original := raw.Original
			if len(removed) > 0 {
				replacementSequence = removed[0].Sequence
				original = joinOriginal(removed)
			}
			s.buffer.AddAt(replacementSequence, stream, original, []byte(action.Output))
		}
	default:
		s.buffer.AddAt(sequence, stream, raw.Original, raw.Original)
	}
	return action, nil
}

func (s *State) takeRawEntries() []BufferEntry {
	entries := slices.Clone(s.rawEntries)
	slices.SortStableFunc(entries, func(a, b BufferEntry) int {
		switch {
		case a.Sequence < b.Sequence:
			return -1
		case a.Sequence > b.Sequence:
			return 1
		default:
			return 0
		}
	})
	s.rawEntries = nil
	return entries
}

func exitReplaceCount(buffer *OrderedBuffer, stream contracts.Stream, count int) int {
	if count > 0 {
		return count
	}
	return buffer.Count(stream)
}

func (s *State) Args() []string {
	return s.command.ArgsForMatching()
}

func (s *State) BufferedLines(stream contracts.Stream) []string {
	return s.buffer.Lines(stream)
}

func (s *State) ExitCode() int {
	return s.exitCode
}

func (s *State) Passthrough() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.passthrough
}

func (s *State) RecoveryEntries() []BufferEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.recovery)
}
