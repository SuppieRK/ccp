package engine_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/engine"
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

	It("drops duplicate keys", func() {
		buffer := engine.NewOrderedBuffer()
		Expect(buffer.Add(contracts.StreamStdout, "x\n")).To(BeTrue())
		Expect(buffer.Add(contracts.StreamStdout, "x\n")).To(BeFalse())
		Expect(buffer.Joined(contracts.StreamStdout)).To(Equal("x\n"))
	})

	It("dedupes within one stream but keeps the same line across different streams", func() {
		buffer := engine.NewOrderedBuffer()

		Expect(buffer.Add(contracts.StreamStdout, "same\n")).To(BeTrue())
		Expect(buffer.Add(contracts.StreamStdout, "same\n")).To(BeFalse())
		Expect(buffer.Add(contracts.StreamStderr, "same\n")).To(BeTrue())

		Expect(buffer.Joined(contracts.StreamStdout)).To(Equal("same\n"))
		Expect(buffer.Joined(contracts.StreamStderr)).To(Equal("same\n"))
	})

	It("skips blank lines", func() {
		buffer := engine.NewOrderedBuffer()

		Expect(buffer.Add(contracts.StreamStdout, "")).To(BeFalse())
		Expect(buffer.Add(contracts.StreamStdout, "   \t")).To(BeFalse())
		Expect(buffer.Add(contracts.StreamStdout, "\n")).To(BeFalse())
		Expect(buffer.Len()).To(Equal(0))
		Expect(buffer.Joined(contracts.StreamStdout)).To(BeEmpty())
	})

	It("strips ANSI sequences before buffering and deduping", func() {
		buffer := engine.NewOrderedBuffer()

		Expect(buffer.Add(contracts.StreamStdout, "\x1b[31mred\x1b[0m\n")).To(BeTrue())
		Expect(buffer.Add(contracts.StreamStdout, "red\n")).To(BeFalse())

		Expect(buffer.Lines(contracts.StreamStdout)).To(Equal([]string{"red\n"}))
		Expect(buffer.Joined(contracts.StreamStdout)).To(Equal("red\n"))
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
})
