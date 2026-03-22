package cli

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Parse", func() {
	DescribeTable("parsing raw execution modes",
		func(args []string) {
			opts := mustParse(args)

			Expect(opts.Raw).To(BeTrue())
			Expect(opts.CommandArgs).To(HaveLen(2))
			Expect(opts.CommandArgs[0]).To(Equal("ls"))
		},
		Entry("raw", []string{"--raw", "ls", "-la"}),
	)

	It("allows confidential without capture raw", func() {
		opts := mustParse([]string{"--confidential", "com.foo", "ls"})

		Expect(opts.ConfidentialRedactions).To(Equal([]string{"com.foo"}))
	})

	DescribeTable("rejecting execution flags for lifecycle commands",
		func(flag string, cmd string) {
			expectParseFailure([]string{flag, cmd})
		},
		Entry("raw for init", "--raw", "init"),
		Entry("raw for gain", "--raw", "gain"),
		Entry("raw for history", "--raw", "history"),
		Entry("raw for upgrade", "--raw", "upgrade"),
		Entry("raw for uninstall", "--raw", "uninstall"),
		Entry("raw for capture", "--raw", "capture"),
		Entry("raw for verify", "--raw", "verify"),
		Entry("raw for repair", "--raw", "repair"),
		Entry("raw for filter", "--raw", "filter"),
	)

	DescribeTable("rejecting confidential for lifecycle commands",
		func(cmd string) {
			expectParseFailure([]string{"--confidential", "com.foo", cmd})
		},
		Entry("init", "init"),
		Entry("capture", "capture"),
		Entry("gain", "gain"),
		Entry("history", "history"),
		Entry("upgrade", "upgrade"),
		Entry("uninstall", "uninstall"),
		Entry("verify", "verify"),
		Entry("repair", "repair"),
		Entry("filter", "filter"),
	)

	DescribeTable("allowing execution flags",
		func(args []string) {
			_, err := Parse(args)
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("raw", []string{"--raw", "ls"}),
		Entry("confidential", []string{"--confidential", "secret", "ls"}),
	)

	It("treats bare double-dash as the end of CCP option parsing", func() {
		opts := mustParse([]string{"--", "echo", "-n"})

		Expect(opts.CommandArgs).To(Equal([]string{"echo", "-n"}))
	})

	DescribeTable("rejecting verbosity flags",
		func(flag string) {
			expectParseFailure([]string{flag, "ls"})
		},
		Entry("v", "-v"),
		Entry("vv", "-vv"),
		Entry("vvv", "-vvv"),
	)

	DescribeTable("parsing help flags",
		func(args []string) {
			opts := mustParse(args)

			Expect(opts.ShowHelp).To(BeTrue())
			Expect(opts.CommandArgs).To(BeEmpty())
		},
		Entry("long", []string{"--help"}),
		Entry("short", []string{"-h"}),
	)

	It("lets help bypass execution flag validation", func() {
		opts := mustParse([]string{"--help", "--raw", "init"})

		Expect(opts.ShowHelp).To(BeTrue())
		Expect(opts.Raw).To(BeTrue())
	})

	Describe("CCP command classification", func() {
		DescribeTable("managed top-level commands",
			func(token string) {
				Expect(IsManagedCommand(token)).To(BeTrue())
			},
			Entry("capture", "capture"),
			Entry("init", "init"),
			Entry("gain", "gain"),
			Entry("history", "history"),
			Entry("verify", "verify"),
			Entry("upgrade", "upgrade"),
			Entry("uninstall", "uninstall"),
			Entry("repair", "repair"),
			Entry("filter", "filter"),
		)

		DescribeTable("non-managed top-level commands",
			func(token string) {
				Expect(IsManagedCommand(token)).To(BeFalse())
			},
			Entry("empty", ""),
			Entry("pwd", "pwd"),
			Entry("bash", "bash"),
		)

		It("skips metrics only for managed wrapped ccp commands", func() {
			Expect(ShouldSkipMetrics("ccp", []string{"ccp", "history"})).To(BeTrue())
			Expect(ShouldSkipMetrics("ccp", []string{"ccp", "repair"})).To(BeTrue())
			Expect(ShouldSkipMetrics("ccp", []string{"ccp", "filter", "new", "demo"})).To(BeTrue())
			Expect(ShouldSkipMetrics("ccp", []string{"ccp", "capture", "--", "echo", "hi"})).To(BeTrue())
			Expect(ShouldSkipMetrics("grep", []string{"grep", "-n", "foo"})).To(BeFalse())
		})

		DescribeTable("describing execution shape",
			func(args []string, expected ExecutionShape) {
				Expect(DescribeExecutionShape(args)).To(Equal(expected))
			},
			Entry("simple command", []string{"echo", "hi"}, ExecutionShape{}),
			Entry("find exec nested ccp", []string{"find", ".", "-type", "f", "-exec", "ccp", "grep", "-nH", "--", "v2", "{}", "+"}, ExecutionShape{
				HasFindExec: true,
				NestedCCP:   true,
			}),
			Entry("shell pipeline with xargs and nested ccp", []string{"bash", "-lc", "find . -print0 | ccp xargs -0 -r ccp grep -nH -- v2"}, ExecutionShape{
				UsesShell:   true,
				HasPipeline: true,
				HasXargs:    true,
				NestedCCP:   true,
			}),
			Entry("shell chain", []string{"sh", "-c", "ccp echo hi && ccp echo bye"}, ExecutionShape{
				UsesShell: true,
				HasChain:  true,
				NestedCCP: true,
			}),
		)
	})
})

func expectParseFailure(args []string) {
	_, err := Parse(args)
	Expect(err).To(HaveOccurred())
}

func mustParse(args []string) Options {
	opts, err := Parse(args)
	Expect(err).NotTo(HaveOccurred())
	return opts
}
