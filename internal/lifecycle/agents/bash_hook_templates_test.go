package agents

import (
	"strings"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("bash hook templates", func() {
	ginkgo.DescribeTable("rendering managed hook scripts",
		func(name string, script string) {
			Expect(strings.HasPrefix(script, "#!/bin/bash\n")).To(BeTrue(), name)
			for _, forbidden := range []string{"jq", "awk", "grep", "cat", "sed", "/usr/bin/env"} {
				Expect(script).NotTo(ContainSubstring(forbidden), name)
			}
		},
		ginkgo.Entry("claude", "claude", claudeHookScriptContent()),
		ginkgo.Entry("codebuddy", "codebuddy", codebuddyHookScriptContent()),
	)
})
