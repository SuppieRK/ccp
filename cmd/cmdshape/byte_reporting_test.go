package main

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("byte-reporting usage", func() {
	It("keeps the full root help inventory with trust-first gain wording", func() {
		usage := usageText()

		for _, expected := range []string{
			"cmdshape — Shape command output. Preserve command truth.",
			"Usage:",
			"Execution flags:",
			"Lifecycle commands:",
			"Notes:",
			"--confidential",
			"capture               Write command.yaml, sequenced streams, and replay output artifacts",
			"init",
			"filter                YAML filter authoring helpers",
			"verify                Replay one fixture directory through the current filter",
			"gain                  Show source, emitted, and net-reduction command-output bytes (--global supported)",
			"history               Show recorded command history (--global supported)",
			"uninstall             Remove selected integrations or fully uninstall cmdshape",
			"Run cmdshape gain after install or init to inspect output shaping on real work.",
			"--raw preserves native output unless --confidential is also used.",
		} {
			Expect(usage).To(ContainSubstring(expected))
		}
		Expect(strings.ToLower(usage)).NotTo(ContainSubstring("token savings"))
	})
})
