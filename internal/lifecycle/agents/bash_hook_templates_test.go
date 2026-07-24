package agents

import (
	"strings"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("bash hook templates", func() {
	ginkgo.DescribeTable("rendering agent titles",
		func(agent string, expected string) {
			Expect(hookAgentTitle(agent)).To(Equal(expected))
		},
		ginkgo.Entry("empty ids stay empty", "", ""),
		ginkgo.Entry("single-word ids are capitalized", "claude", "Claude"),
		ginkgo.Entry("existing mixed case preserves the suffix", "codeBuddy", "CodeBuddy"),
	)

	ginkgo.DescribeTable("rendering managed hook scripts",
		func(name string, script string) {
			Expect(strings.HasPrefix(script, "#!/bin/bash\n")).To(BeTrue(), name)
			for _, forbidden := range []string{"jq", "awk", "grep", "cat", "sed", "/usr/bin/env"} {
				Expect(script).NotTo(ContainSubstring(forbidden), name)
			}
		},
		ginkgo.Entry("claude", "claude", bashRewriteHookScriptContent("claude", "cmdshape-claude-hook.log")),
		ginkgo.Entry("codebuddy", "codebuddy", bashRewriteHookScriptContent("codebuddy", "cmdshape-codebuddy-hook.log")),
	)
})
