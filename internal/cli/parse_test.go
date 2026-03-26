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

	DescribeTable("rejecting missing flag values",
		func(args []string, expected string) {
			_, err := Parse(args)

			Expect(err).To(MatchError(expected))
		},
		Entry("missing confidential value", []string{"--confidential"}, "missing value for --confidential"),
	)

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

	It("allows empty confidential redactions for managed commands", func() {
		opts := mustParse([]string{"--confidential", " , , ", "init"})

		Expect(opts.CommandArgs).To(Equal([]string{"init"}))
		Expect(opts.ConfidentialRedactions).To(BeEmpty())
	})

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

	DescribeTable("normalizing confidential redactions",
		func(raw string, expected []string) {
			Expect(parseConfidentialRedactions(raw)).To(Equal(expected))
		},
		Entry("empty string", "", nil),
		Entry("whitespace only", "   ", nil),
		Entry("duplicates and blanks", " secret, secret , ,token ", []string{"secret", "token"}),
	)

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

		DescribeTable("classifying managed argument slices",
			func(args []string, expected bool) {
				Expect(IsManagedArgs(args)).To(Equal(expected))
			},
			Entry("nil args", nil, false),
			Entry("empty args", []string{}, false),
			Entry("history command", []string{"history"}, true),
			Entry("filter command", []string{"filter", "new"}, true),
			Entry("wrapped command", []string{"grep", "-n"}, false),
		)

		DescribeTable("skipping metrics only for managed wrapped ccp commands",
			func(tool string, args []string, expected bool) {
				Expect(ShouldSkipMetrics(tool, args)).To(Equal(expected))
			},
			Entry("history command", "ccp", []string{"ccp", "history"}, true),
			Entry("repair command", "ccp", []string{"ccp", "repair"}, true),
			Entry("filter command", "ccp", []string{"ccp", "filter", "new", "demo"}, true),
			Entry("capture command", "ccp", []string{"ccp", "capture", "--", "echo", "hi"}, true),
			Entry("non-ccp tool", "grep", []string{"grep", "-n", "foo"}, false),
			Entry("ccp without subcommand", "ccp", []string{"ccp"}, false),
			Entry("ccp wrapped execution", "ccp", []string{"ccp", "echo", "hi"}, false),
		)

		DescribeTable("describing execution shape",
			func(args []string, expected ExecutionShape) {
				Expect(DescribeExecutionShape(args)).To(Equal(expected))
			},
			Entry("empty args", nil, ExecutionShape{}),
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
			Entry("bare shell control flag", []string{"bash", "-c"}, ExecutionShape{
				UsesShell: true,
			}),
			Entry("shell find exec only", []string{"bash", "-lc", "find . -name '*.go' -exec grep -n foo {} +"}, ExecutionShape{
				UsesShell:   true,
				HasFindExec: true,
			}),
			Entry("shell xargs only", []string{"zsh", "-c", "printf '%s\n' file | xargs cat"}, ExecutionShape{
				UsesShell:   true,
				HasPipeline: true,
				HasXargs:    true,
			}),
			Entry("non-shell find followed immediately by exec still marks find-exec", []string{"wrapper", "find", "-exec", "grep", "foo"}, ExecutionShape{
				HasFindExec: true,
			}),
			Entry("non-shell find without exec", []string{"echo", "find", "."}, ExecutionShape{}),
			Entry("non-shell c lookalike", []string{"python", "-c", "print('hi')"}, ExecutionShape{}),
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
