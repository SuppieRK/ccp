package engine

import (
	"strings"

	"go-command-compression-proxy/internal/contracts"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type recordingFilter struct{}

func (f *recordingFilter) PrepareCommand(command contracts.Command) (contracts.Command, error) {
	return command, nil
}

func (f *recordingFilter) Dispatch(command contracts.Command) string {
	return command.Tool
}

func (f *recordingFilter) OnStdout(line string, context contracts.Context) contracts.Action {
	_ = line
	_ = context
	return contracts.Action{Kind: contracts.ActionKeep}
}

func (f *recordingFilter) OnStderr(_ string, _ contracts.Context) contracts.Action {
	return contracts.Action{Kind: contracts.ActionReplace, Output: "E\n"}
}

func (f *recordingFilter) OnStdoutExit(context contracts.Context) contracts.Action {
	return contracts.Action{
		Kind:   contracts.ActionReplace,
		Output: "S\n",
	}
}

type exitNoopFilter struct{}

func (f *exitNoopFilter) PrepareCommand(command contracts.Command) (contracts.Command, error) {
	return command, nil
}

func (f *exitNoopFilter) Dispatch(command contracts.Command) string {
	return command.Tool
}

func (f *exitNoopFilter) OnStdout(line string, _ contracts.Context) contracts.Action {
	_ = line
	return contracts.Action{Kind: contracts.ActionKeep}
}

func (f *exitNoopFilter) OnStderr(line string, _ contracts.Context) contracts.Action {
	_ = line
	return contracts.Action{Kind: contracts.ActionKeep}
}

func (f *exitNoopFilter) OnStdoutExit(_ contracts.Context) contracts.Action {
	return contracts.Action{Kind: contracts.ActionKeep}
}

type combinedExitFilter struct{}

func (f *combinedExitFilter) PrepareCommand(command contracts.Command) (contracts.Command, error) {
	return command, nil
}

func (f *combinedExitFilter) Dispatch(command contracts.Command) string {
	return command.Tool
}

func (f *combinedExitFilter) OnStdout(line string, _ contracts.Context) contracts.Action {
	_ = line
	return contracts.Action{Kind: contracts.ActionKeep}
}

func (f *combinedExitFilter) OnStderr(line string, _ contracts.Context) contracts.Action {
	_ = line
	return contracts.Action{Kind: contracts.ActionKeep}
}

func (f *combinedExitFilter) OnStdoutExit(context contracts.Context) contracts.Action {
	return contracts.Action{
		Kind:   contracts.ActionReplace,
		Stream: contracts.StreamCombined,
		Output: "S\n",
	}
}

type multiExitFilter struct {
	exitNoopFilter
}

func (f *multiExitFilter) OnStdoutExitActions(contracts.Context) []contracts.Action {
	return []contracts.Action{
		{Kind: contracts.ActionReplace, Stream: contracts.StreamStdout, Output: "S\n"},
		{Kind: contracts.ActionReplace, Stream: contracts.StreamStderr, Output: "E\n"},
	}
}

type scriptedFilter struct {
	stdoutAction contracts.Action
	stderrAction contracts.Action
	exitAction   contracts.Action
}

type interleavingFilter struct{}

func (f *interleavingFilter) PrepareCommand(command contracts.Command) (contracts.Command, error) {
	return command, nil
}

func (f *interleavingFilter) Dispatch(command contracts.Command) string {
	return command.Tool
}

func (f *interleavingFilter) OnStdout(line string, _ contracts.Context) contracts.Action {
	switch {
	case strings.HasPrefix(line, "drop"):
		return contracts.Action{Kind: contracts.ActionIgnore}
	case strings.HasPrefix(line, "replace"):
		return contracts.Action{Kind: contracts.ActionReplace, Output: "replacement\n"}
	default:
		return contracts.Action{Kind: contracts.ActionKeep}
	}
}

func (f *interleavingFilter) OnStderr(line string, _ contracts.Context) contracts.Action {
	return f.OnStdout(line, nil)
}

func (f *interleavingFilter) OnStdoutExit(contracts.Context) contracts.Action {
	return contracts.Action{Kind: contracts.ActionKeep}
}

func (f *scriptedFilter) PrepareCommand(command contracts.Command) (contracts.Command, error) {
	return command, nil
}

func (f *scriptedFilter) Dispatch(command contracts.Command) string {
	return command.Tool
}

func (f *scriptedFilter) OnStdout(string, contracts.Context) contracts.Action {
	return f.stdoutAction
}

func (f *scriptedFilter) OnStderr(string, contracts.Context) contracts.Action {
	return f.stderrAction
}

func (f *scriptedFilter) OnStdoutExit(contracts.Context) contracts.Action {
	return f.exitAction
}

func entryLines(entries []BufferEntry) []string {
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, entry.Line)
	}
	return lines
}

func entryStreams(entries []BufferEntry) []contracts.Stream {
	streams := make([]contracts.Stream, 0, len(entries))
	for _, entry := range entries {
		streams = append(streams, entry.Stream)
	}
	return streams
}

var _ = Describe("Engine integration", func() {
	Context("when a matching filter is registered", func() {
		var (
			registry *Registry
			runtime  *Engine
			state    *State
		)

		BeforeEach(func() {
			registry = NewRegistry()
			registry.Register("demo", &recordingFilter{})
			runtime = NewEngine(registry)
			state = runtime.Start(contracts.Command{
				CommandID: "cmd-1",
				RawInput:  "demo --flag",
				Args:      []string{"demo", "--flag"},
				Tool:      "demo",
				Dispatch:  "recording",
			})
		})

		It("keeps command metadata on the execution state", func() {
			command := state.command
			Expect(command.CommandID).To(Equal("cmd-1"))
			Expect(command.RawInput).To(Equal("demo --flag"))
			Expect(command.Tool).To(Equal("demo"))
			Expect(command.Dispatch).To(Equal("recording"))
		})

		It("uses filter actions for stdout, stderr, and exit handling", func() {
			Expect(state.Stdout("one\n")).To(BeEmpty())
			Expect(state.Stderr("warn\n")).To(BeEmpty())
			entries := state.Exit(0)
			Expect(entryLines(entries)).To(Equal([]string{"S\n", "E\n"}))
			Expect(entryStreams(entries)).To(Equal([]contracts.Stream{
				contracts.StreamStdout,
				contracts.StreamStderr,
			}))
			Expect(entries[0].Original).To(Equal([]byte("one\n")))
			Expect(entries[0].Transformed).To(Equal([]byte("S\n")))
			Expect(entries[1].Original).To(Equal([]byte("warn\n")))
		})
	})

	Context("when no filter matches", func() {
		var state *State

		BeforeEach(func() {
			runtime := NewEngine(NewRegistry())
			state = runtime.Start(contracts.Command{
				CommandID: "cmd-2",
				RawInput:  "unknown",
				Args:      []string{"unknown"},
			})
		})

		It("falls back to passthrough", func() {
			Expect(state.command.CommandID).To(Equal("cmd-2"))
			Expect(entryLines(state.Stdout("hello\n"))).To(Equal([]string{"hello\n"}))
			Expect(entryLines(state.Stderr("warn\n"))).To(Equal([]string{"warn\n"}))
			Expect(state.Exit(0)).To(BeEmpty())
		})
	})

	Context("when retained lines remain buffered at exit", func() {
		var state *State

		BeforeEach(func() {
			registry := NewRegistry()
			registry.Register("buffered", &exitNoopFilter{})
			runtime := NewEngine(registry)
			state = runtime.Start(contracts.Command{
				CommandID: "cmd-3",
				RawInput:  "buffered",
				Args:      []string{"buffered"},
				Tool:      "buffered",
			})
		})

		It("flushes them in original sequence order", func() {
			Expect(state.Stdout("out-1\n")).To(BeEmpty())
			Expect(state.Stderr("err-1\n")).To(BeEmpty())
			Expect(state.Stdout("out-2\n")).To(BeEmpty())

			entries := state.Exit(0)
			Expect(entryLines(entries)).To(Equal([]string{"out-1\n", "err-1\n", "out-2\n"}))
			Expect(entryStreams(entries)).To(Equal([]contracts.Stream{
				contracts.StreamStdout,
				contracts.StreamStderr,
				contracts.StreamStdout,
			}))
			Expect(entries[0].Sequence).To(Equal(uint64(0)))
			Expect(entries[1].Sequence).To(Equal(uint64(1)))
			Expect(entries[2].Sequence).To(Equal(uint64(2)))
			Expect(entries[0].Original).To(Equal([]byte("out-1\n")))
			Expect(entries[0].Transformed).To(Equal([]byte("out-1\n")))
			Expect(entries[0].Newline).To(BeTrue())
		})
	})

	Context("when ignored stderr lines arrive after retained stderr", func() {
		var state *State

		BeforeEach(func() {
			registry := NewRegistry()
			registry.Register("buffered", &exitNoopFilter{})
			runtime := NewEngine(registry)
			state = runtime.Start(contracts.Command{
				CommandID: "cmd-4",
				RawInput:  "buffered",
				Args:      []string{"buffered"},
				Tool:      "buffered",
			})
			Expect(state.Stderr("err-1\n")).To(BeEmpty())
		})

		Context("when the ignored line is blank", func() {
			It("preserves both retained records", func() {
				Expect(state.Stderr("\n")).To(BeEmpty())
				Expect(entryLines(state.Exit(0))).To(Equal([]string{"err-1\n", "\n"}))
			})
		})

		Context("when the ignored line is a duplicate", func() {
			It("preserves both copies", func() {
				Expect(state.Stderr("err-1\n")).To(BeEmpty())
				Expect(entryLines(state.Exit(0))).To(Equal([]string{"err-1\n", "err-1\n"}))
			})
		})
	})

	Context("when exit handling targets the combined stream", func() {
		It("falls back to raw ordered output when both native streams contributed", func() {
			registry := NewRegistry()
			registry.Register("combined", &combinedExitFilter{})
			runtime := NewEngine(registry)
			state := runtime.Start(contracts.Command{
				CommandID: "cmd-5",
				RawInput:  "combined",
				Args:      []string{"combined"},
				Tool:      "combined",
			})

			Expect(state.Stdout("out-1\n")).To(BeEmpty())
			Expect(state.Stderr("err-1\n")).To(BeEmpty())

			entries := state.Exit(0)
			Expect(entryLines(entries)).To(Equal([]string{"out-1\n", "err-1\n"}))
			Expect(entryStreams(entries)).To(Equal([]contracts.Stream{
				contracts.StreamStdout,
				contracts.StreamStderr,
			}))
			Expect(state.Passthrough()).To(BeTrue())
		})

		It("retains the contributing stream for single-stream combined output", func() {
			registry := NewRegistry()
			registry.Register("combined", &combinedExitFilter{})
			state := NewEngine(registry).Start(contracts.Command{
				Args: []string{"combined"},
				Tool: "combined",
			})

			Expect(state.Stderr("err-only\n")).To(BeEmpty())
			entries := state.Exit(0)
			Expect(entryLines(entries)).To(Equal([]string{"S\n"}))
			Expect(entryStreams(entries)).To(Equal([]contracts.Stream{contracts.StreamStderr}))
		})
	})

	It("applies independent stdout and stderr exit actions", func() {
		registry := NewRegistry()
		registry.Register("multi", &multiExitFilter{})
		state := NewEngine(registry).Start(contracts.Command{
			Args: []string{"multi"},
			Tool: "multi",
		})

		Expect(state.Stdout("long stdout line\n")).To(BeEmpty())
		Expect(state.Stderr("long stderr line\n")).To(BeEmpty())

		entries := state.Exit(0)
		Expect(entryLines(entries)).To(Equal([]string{"S\n", "E\n"}))
		Expect(entryStreams(entries)).To(Equal([]contracts.Stream{
			contracts.StreamStdout,
			contracts.StreamStderr,
		}))
		Expect(state.Passthrough()).To(BeFalse())
	})

	Context("when action helpers are used directly", func() {
		var state *State

		BeforeEach(func() {
			registry := NewRegistry()
			registry.Register("scripted", &scriptedFilter{
				stdoutAction: contracts.Action{Kind: contracts.ActionEmit},
				stderrAction: contracts.Action{Kind: contracts.ActionIgnore},
				exitAction: contracts.Action{
					Kind:         contracts.ActionReplace,
					Stream:       contracts.StreamStdout,
					ReplaceCount: 0,
					Output:       "s\n",
				},
			})
			runtime := NewEngine(registry)
			state = runtime.Start(contracts.Command{
				CommandID: "cmd-6",
				RawInput:  "scripted --flag",
				Args:      []string{"scripted", "--flag"},
				Tool:      "scripted",
			})
		})

		It("returns the action and emitted entries for stdout", func() {
			action, entries := state.StdoutAction("out\n")

			Expect(action.Kind).To(Equal(contracts.ActionEmit))
			Expect(entryLines(entries)).To(Equal([]string{"out\n"}))
			Expect(entryStreams(entries)).To(Equal([]contracts.Stream{contracts.StreamStdout}))
			Expect(entryLines(state.Exit(0))).To(Equal([]string{"s\n"}))
		})

		It("streams emit actions without waiting for exit", func() {
			entries := state.Stdout("first\n")

			Expect(entryLines(entries)).To(Equal([]string{"first\n"}))
			Expect(state.Passthrough()).To(BeFalse())
		})

		It("returns the action and no entries for ignored stderr", func() {
			action, entries := state.StderrAction("warn\n")

			Expect(action.Kind).To(Equal(contracts.ActionIgnore))
			Expect(entries).To(BeNil())
		})

		It("returns the exit action and buffered summary output", func() {
			registry := NewRegistry()
			registry.Register("scripted-exit", &scriptedFilter{
				stdoutAction: contracts.Action{Kind: contracts.ActionKeep},
				exitAction: contracts.Action{
					Kind:         contracts.ActionReplace,
					Stream:       contracts.StreamStdout,
					ReplaceCount: 0,
					Output:       "s\n",
				},
			})
			runtime := NewEngine(registry)
			exitState := runtime.Start(contracts.Command{
				CommandID: "cmd-6-exit",
				Args:      []string{"scripted-exit"},
				Tool:      "scripted-exit",
			})
			Expect(exitState.Stdout("kept\n")).To(BeEmpty())

			action, entries := exitState.ExitAction(9)

			Expect(action.Kind).To(Equal(contracts.ActionReplace))
			Expect(action.ReplaceCount).To(BeZero())
			Expect(entryLines(entries)).To(Equal([]string{"s\n"}))
			Expect(entryStreams(entries)).To(Equal([]contracts.Stream{contracts.StreamStdout}))
			Expect(exitState.ExitCode()).To(Equal(9))
		})

		It("exposes the original command arguments", func() {
			Expect(state.Args()).To(Equal([]string{"scripted", "--flag"}))
		})
	})

	Context("when replace actions omit a replace count", func() {
		It("replaces the current line by default", func() {
			registry := NewRegistry()
			registry.Register("replace-default", &scriptedFilter{
				stdoutAction: contracts.Action{
					Kind:   contracts.ActionReplace,
					Output: "x\n",
				},
			})
			runtime := NewEngine(registry)
			state := runtime.Start(contracts.Command{
				CommandID: "cmd-7",
				Args:      []string{"replace-default"},
				Tool:      "replace-default",
			})

			action, entries := state.StdoutAction("before\n")

			Expect(action.Kind).To(Equal(contracts.ActionReplace))
			Expect(entries).To(BeNil())
			Expect(state.BufferedLines(contracts.StreamStdout)).To(Equal([]string{"x\n"}))
			Expect(entryLines(state.Exit(0))).To(Equal([]string{"x\n"}))
		})
	})

	It("flushes raw ordered records and switches permanently to passthrough at the candidate cap", func() {
		filter := &scriptedFilter{
			stdoutAction: contracts.Action{Kind: contracts.ActionKeep},
			stderrAction: contracts.Action{Kind: contracts.ActionKeep},
			exitAction:   contracts.Action{Kind: contracts.ActionKeep},
		}
		state := newStateWithLimit(contracts.Command{Args: []string{"bounded"}, Tool: "bounded"}, filter, 5)

		Expect(state.Stdout("abc")).To(BeNil())
		entries := state.Stderr("def")
		Expect(entryLines(entries)).To(Equal([]string{"abc", "def"}))
		Expect(entryStreams(entries)).To(Equal([]contracts.Stream{
			contracts.StreamStdout,
			contracts.StreamStderr,
		}))
		Expect(state.Passthrough()).To(BeTrue())

		entries = state.Stdout("ghi")
		Expect(entryLines(entries)).To(Equal([]string{"ghi"}))
		Expect(entries[0].Original).To(Equal([]byte("ghi")))
		Expect(entries[0].Transformed).To(Equal([]byte("ghi")))
		Expect(state.Exit(0)).To(BeNil())
	})

	It("honors an explicit stderr exit target", func() {
		filter := &scriptedFilter{
			stdoutAction: contracts.Action{Kind: contracts.ActionKeep},
			stderrAction: contracts.Action{Kind: contracts.ActionKeep},
			exitAction: contracts.Action{
				Kind:   contracts.ActionReplace,
				Stream: contracts.StreamStderr,
				Output: "e\n",
			},
		}
		state := newState(contracts.Command{Args: []string{"stderr-exit"}, Tool: "stderr-exit"}, filter)
		Expect(state.Stdout("out\n")).To(BeNil())
		Expect(state.Stderr("err\n")).To(BeNil())

		entries := state.Exit(1)
		Expect(entryLines(entries)).To(Equal([]string{"out\n", "e\n"}))
		Expect(entryStreams(entries)).To(Equal([]contracts.Stream{
			contracts.StreamStdout,
			contracts.StreamStderr,
		}))
	})

	It("preserves positions and duplicates through keep, replace, and ignore interleavings", func() {
		state := newState(
			contracts.Command{Args: []string{"interleave"}, Tool: "interleave"},
			&interleavingFilter{},
		)

		Expect(state.Stdout("same\n")).To(BeNil())
		Expect(state.Stderr("drop stderr\n")).To(BeNil())
		Expect(state.Stdout("replace stdout\n")).To(BeNil())
		Expect(state.Stderr("same\n")).To(BeNil())
		Expect(state.Stdout("same\n")).To(BeNil())

		entries := state.Exit(0)
		Expect(entryLines(entries)).To(Equal([]string{
			"same\n",
			"replacement\n",
			"same\n",
			"same\n",
		}))
		Expect(entryStreams(entries)).To(Equal([]contracts.Stream{
			contracts.StreamStdout,
			contracts.StreamStdout,
			contracts.StreamStderr,
			contracts.StreamStdout,
		}))
		Expect(entries[1].Sequence).To(Equal(uint64(2)))
		Expect(entries[1].Original).To(Equal([]byte("replace stdout\n")))
		Expect(entries[1].Transformed).To(Equal([]byte("replacement\n")))
	})

	It("retains ANSI, invalid UTF-8, CR, and final-newline state", func() {
		state := newState(
			contracts.Command{Args: []string{"bytes"}, Tool: "bytes"},
			&exitNoopFilter{},
		)
		first := string([]byte{0x1b, '[', '3', '1', 'm', 0xff, '\r'})
		Expect(state.Stdout(first)).To(BeNil())
		Expect(state.Stderr("tail-without-newline")).To(BeNil())

		entries := state.Exit(0)
		Expect(entries).To(HaveLen(2))
		Expect(entries[0].Original).To(Equal([]byte(first)))
		Expect(entries[0].Transformed).To(Equal([]byte(first)))
		Expect(entries[0].Newline).To(BeFalse())
		Expect(entries[1].Original).To(Equal([]byte("tail-without-newline")))
		Expect(entries[1].Newline).To(BeFalse())
	})

	DescribeTable("selects transformed output only when it is strictly smaller",
		func(replacement string, expected string, expectPassthrough bool) {
			filter := &scriptedFilter{
				stdoutAction: contracts.Action{Kind: contracts.ActionReplace, Output: replacement},
				exitAction:   contracts.Action{Kind: contracts.ActionKeep},
			}
			state := newState(
				contracts.Command{Args: []string{"size-gate"}, Tool: "size-gate"},
				filter,
			)

			Expect(state.Stdout("abc")).To(BeNil())
			Expect(entryLines(state.Exit(0))).To(Equal([]string{expected}))
			Expect(state.Passthrough()).To(Equal(expectPassthrough))
		},
		Entry("accepts a smaller candidate", "x", "x", false),
		Entry("keeps native bytes on equality", "xyz", "abc", true),
		Entry("keeps native bytes on expansion", "expanded", "abc", true),
	)

	It("panics when constructed with a nil registry", func() {
		Expect(func() {
			NewEngine(nil)
		}).To(Panic())
	})
})
