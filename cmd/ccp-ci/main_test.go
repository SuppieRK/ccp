package main

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ccp-ci", func() {
	It("scopes fixtures to the selected tool by joining the root and tool name", func() {
		root := GinkgoT().TempDir()
		toolDir, err := resolveFixturesRoot(root, "grep")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.MkdirAll(toolDir, 0o755)).To(Succeed())
		Expect(filepath.Join(root, "grep")).To(Equal(toolDir))
	})

	It("rejects tool names that escape the benchmark fixture root", func() {
		root := GinkgoT().TempDir()

		_, err := resolveFixturesRoot(root, "../outside")

		Expect(err).To(MatchError(ContainSubstring("single tool directory name")))
	})
})
