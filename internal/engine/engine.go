package engine

import (
	"sync"

	"go-command-compression-proxy/internal/contracts"
)

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
	filter  contracts.Filter
	command contracts.Command
	buffer  *OrderedBuffer
	mu      sync.Mutex
}

func newState(command contracts.Command, filter contracts.Filter) *State {
	return &State{
		filter:  filter,
		command: command,
		buffer:  NewOrderedBuffer(),
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

func (s *State) Exit() []BufferEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyExit(s.filter.OnStdoutExit(s))
	return s.buffer.Entries()
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

func (s *State) ExitAction() (contracts.Action, []BufferEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	action := s.filter.OnStdoutExit(s)
	s.applyExit(action)
	return action, s.buffer.Entries()
}

func (s *State) applyExit(action contracts.Action) {
	switch action.Kind {
	case contracts.ActionKeep, "":
		return
	case contracts.ActionEmit:
		return
	case contracts.ActionIgnore:
		s.buffer.RemoveLast(contracts.StreamStdout, s.buffer.Count(contracts.StreamStdout))
		return
	case contracts.ActionReplace:
		s.buffer.RemoveLast(contracts.StreamStdout, exitReplaceCount(s.buffer, contracts.StreamStdout, action.ReplaceCount))
		s.buffer.Add(contracts.StreamStdout, action.Output)
		return
	default:

		return
	}
}

func (s *State) applyAction(stream contracts.Stream, line string, action contracts.Action) (contracts.Action, []BufferEntry) {
	added := s.buffer.Add(stream, line)
	switch action.Kind {
	case contracts.ActionKeep:
		return action, nil
	case contracts.ActionEmit:
		if added {
			s.buffer.RemoveLast(stream, 1)
		}
		return action, []BufferEntry{{Stream: stream, Line: line}}
	case contracts.ActionIgnore:
		if added {
			s.buffer.RemoveLast(stream, 1)
		}
		return action, nil
	case contracts.ActionReplace:
		count := action.ReplaceCount
		if count < 1 {
			count = 1
		}
		if added {
			s.buffer.RemoveLast(stream, count)
		}
		if action.Output == "" {
			return action, nil
		}
		return action, []BufferEntry{{Stream: stream, Line: action.Output}}
	default:
		return action, nil
	}
}

func exitReplaceCount(buffer *OrderedBuffer, stream contracts.Stream, count int) int {
	if count > 0 {
		return count
	}
	return buffer.Count(stream)
}

func (s *State) Args() []string {
	return s.command.Args
}

func (s *State) BufferedLines(stream contracts.Stream) []string {
	return s.buffer.Lines(stream)
}
