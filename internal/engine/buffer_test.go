package engine_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/SuppieRK/cmdshape/internal/contracts"
	"github.com/SuppieRK/cmdshape/internal/engine"
)

var _ = Describe("OrderedBuffer", func() {
	It("preserves insertion ordering", func() {
		buffer := engine.NewOrderedBuffer()
		Expect(buffer.Add(contracts.StreamStdout, "a\n")).To(BeTrue())
		Expect(buffer.Add(contracts.StreamStderr, "b\n")).To(BeTrue())

		Expect(buffer.Lines(contracts.StreamStdout)).To(Equal([]string{"a\n"}))
		Expect(buffer.Lines(contracts.StreamStderr)).To(Equal([]string{"b\n"}))
		Expect(buffer.Joined(contracts.StreamStdout)).To(Equal("a\n"))
		Expect(buffer.Joined(contracts.StreamStderr)).To(Equal("b\n"))
	})

	It("preserves adjacent duplicate records within one stream", func() {
		buffer := engine.NewOrderedBuffer()
		Expect(buffer.Add(contracts.StreamStdout, "x\n")).To(BeTrue())
		Expect(buffer.Add(contracts.StreamStdout, "x\n")).To(BeTrue())
		Expect(buffer.Joined(contracts.StreamStdout)).To(Equal("x\nx\n"))
	})

	It("preserves multiplicity independently across streams", func() {
		buffer := engine.NewOrderedBuffer()

		Expect(buffer.Add(contracts.StreamStdout, "same\n")).To(BeTrue())
		Expect(buffer.Add(contracts.StreamStdout, "same\n")).To(BeTrue())
		Expect(buffer.Add(contracts.StreamStderr, "same\n")).To(BeTrue())

		Expect(buffer.Joined(contracts.StreamStdout)).To(Equal("same\nsame\n"))
		Expect(buffer.Joined(contracts.StreamStderr)).To(Equal("same\n"))
	})

	It("preserves zero-length and blank records", func() {
		buffer := engine.NewOrderedBuffer()

		Expect(buffer.Add(contracts.StreamStdout, "")).To(BeTrue())
		Expect(buffer.Add(contracts.StreamStdout, "   \t")).To(BeTrue())
		Expect(buffer.Add(contracts.StreamStdout, "\n")).To(BeTrue())
		Expect(buffer.Len()).To(Equal(3))
		Expect(buffer.Joined(contracts.StreamStdout)).To(Equal("   \t\n"))
	})

	It("preserves ANSI sequences as original output bytes", func() {
		buffer := engine.NewOrderedBuffer()

		Expect(buffer.Add(contracts.StreamStdout, "\x1b[31mred\x1b[0m\n")).To(BeTrue())
		Expect(buffer.Add(contracts.StreamStdout, "red\n")).To(BeTrue())

		Expect(buffer.Lines(contracts.StreamStdout)).To(Equal([]string{"\x1b[31mred\x1b[0m\n", "red\n"}))
		Expect(buffer.Joined(contracts.StreamStdout)).To(Equal("\x1b[31mred\x1b[0m\nred\n"))
	})

	It("records sequence, original bytes, transformed bytes, and newline state", func() {
		buffer := engine.NewOrderedBuffer()
		buffer.AddAt(7, contracts.StreamStderr, []byte("before\r"), []byte("after"))

		Expect(buffer.Entries()).To(Equal([]engine.BufferEntry{{
			Sequence:    7,
			Stream:      contracts.StreamStderr,
			Original:    []byte("before\r"),
			Transformed: []byte("after"),
			Line:        "after",
			Newline:     false,
		}}))
	})

	It("recomputes joined output after add and clear", func() {
		buffer := engine.NewOrderedBuffer()
		Expect(buffer.Add(contracts.StreamStdout, "a\n")).To(BeTrue())
		Expect(buffer.Joined(contracts.StreamStdout)).To(Equal("a\n"))

		Expect(buffer.Add(contracts.StreamStdout, "b\n")).To(BeTrue())
		Expect(buffer.Joined(contracts.StreamStdout)).To(Equal("a\nb\n"))

		buffer.Clear()
		Expect(buffer.Joined(contracts.StreamStdout)).To(BeEmpty())
	})

	It("removes the last matching entries from a single stream", func() {
		buffer := engine.NewOrderedBuffer()
		Expect(buffer.Add(contracts.StreamStdout, "out-1\n")).To(BeTrue())
		Expect(buffer.Add(contracts.StreamStderr, "err-1\n")).To(BeTrue())
		Expect(buffer.Add(contracts.StreamStdout, "out-2\n")).To(BeTrue())

		Expect(buffer.RemoveLast(contracts.StreamStdout, 1)).To(Equal(1))
		Expect(buffer.Lines(contracts.StreamCombined)).To(Equal([]string{"out-1\n", "err-1\n"}))
	})

	It("removes entries across streams for the combined stream and re-enables re-adding", func() {
		buffer := engine.NewOrderedBuffer()
		Expect(buffer.Add(contracts.StreamStdout, "out-1\n")).To(BeTrue())
		Expect(buffer.Add(contracts.StreamStderr, "err-1\n")).To(BeTrue())

		Expect(buffer.RemoveLast(contracts.StreamCombined, 2)).To(Equal(2))
		Expect(buffer.Len()).To(BeZero())
		Expect(buffer.Add(contracts.StreamStdout, "out-1\n")).To(BeTrue())
	})

	It("returns zero when removing from an empty buffer or with a non-positive count", func() {
		buffer := engine.NewOrderedBuffer()
		Expect(buffer.RemoveLast(contracts.StreamStdout, 0)).To(BeZero())
		Expect(buffer.Add(contracts.StreamStdout, "out-1\n")).To(BeTrue())
		Expect(buffer.RemoveLast(contracts.StreamStdout, -1)).To(BeZero())
		Expect(buffer.Joined(contracts.StreamStdout)).To(Equal("out-1\n"))
	})
})
