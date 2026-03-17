package engine

import (
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("stripANSI", func() {
	ginkgo.DescribeTable("removes ANSI control sequences from output",
		func(input string, expected string) {
			Expect(stripANSI(input)).To(Equal(expected))
		},
		ginkgo.Entry("CSI color", "\x1b[31merror\x1b[0m\n", "error\n"),
		ginkgo.Entry("multiple CSI sequences", "\x1b[1m\x1b[32msuccess\x1b[0m\n", "success\n"),
		ginkgo.Entry("plain text", "no ansi\n", "no ansi\n"),
	)
})
