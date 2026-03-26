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
	return contracts.Action{Kind: contracts.ActionReplace, Output: "ERR:warn\n"}
}

func (f *recordingFilter) OnStdoutExit(context contracts.Context) contracts.Action {
	return contracts.Action{
		Kind:   contracts.ActionReplace,
		Output: "summary: " + strings.Join(context.BufferedLines(contracts.StreamStdout), ""),
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
		Output: "summary: " + strings.Join(context.BufferedLines(contracts.StreamCombined), ""),
	}
}

type scriptedFilter struct {
	stdoutAction contracts.Action
	stderrAction contracts.Action
	exitAction   contracts.Action
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
			Expect(state.Stderr("warn\n")).To(Equal([]BufferEntry{{
				Stream: contracts.StreamStderr,
				Line:   "ERR:warn\n",
			}}))
			Expect(state.Exit(0)).To(Equal([]BufferEntry{{
				Stream: contracts.StreamStdout,
				Line:   "summary: one\n",
			}}))
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
			Expect(state.Stdout("hello\n")).To(Equal([]BufferEntry{{
				Stream: contracts.StreamStdout,
				Line:   "hello\n",
			}}))
			Expect(state.Stderr("warn\n")).To(Equal([]BufferEntry{{
				Stream: contracts.StreamStderr,
				Line:   "warn\n",
			}}))
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

			Expect(state.Exit(0)).To(Equal([]BufferEntry{
				{Stream: contracts.StreamStdout, Line: "out-1\n"},
				{Stream: contracts.StreamStderr, Line: "err-1\n"},
				{Stream: contracts.StreamStdout, Line: "out-2\n"},
			}))
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
			It("keeps the previously retained line", func() {
				Expect(state.Stderr("\n")).To(BeEmpty())
				Expect(state.Exit(0)).To(Equal([]BufferEntry{
					{Stream: contracts.StreamStderr, Line: "err-1\n"},
				}))
			})
		})

		Context("when the ignored line is a duplicate", func() {
			It("keeps the previously retained line", func() {
				Expect(state.Stderr("err-1\n")).To(BeEmpty())
				Expect(state.Exit(0)).To(Equal([]BufferEntry{
					{Stream: contracts.StreamStderr, Line: "err-1\n"},
				}))
			})
		})
	})

	Context("when exit handling targets the combined stream", func() {
		It("replaces the full retained combined output and emits the summary on stdout", func() {
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

			Expect(state.Exit(0)).To(Equal([]BufferEntry{{
				Stream: contracts.StreamStdout,
				Line:   "summary: out-1\nerr-1\n",
			}}))
		})
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
					Output:       "summary\n",
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
			Expect(entries).To(Equal([]BufferEntry{{
				Stream: contracts.StreamStdout,
				Line:   "out\n",
			}}))
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
					Output:       "summary\n",
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
			Expect(entries).To(Equal([]BufferEntry{{
				Stream: contracts.StreamStdout,
				Line:   "summary\n",
			}}))
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
					Output: "rewritten\n",
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
			Expect(entries).To(Equal([]BufferEntry{{
				Stream: contracts.StreamStdout,
				Line:   "rewritten\n",
			}}))
			Expect(state.BufferedLines(contracts.StreamStdout)).To(BeEmpty())
		})
	})

	It("panics when constructed with a nil registry", func() {
		Expect(func() {
			NewEngine(nil)
		}).To(Panic())
	})
})
