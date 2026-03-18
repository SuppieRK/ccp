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

	It("panics when constructed with a nil registry", func() {
		Expect(func() {
			NewEngine(nil)
		}).To(Panic())
	})
})
