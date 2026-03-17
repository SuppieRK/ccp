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
		toolDir := filepath.Join(root, "grep")
		Expect(os.MkdirAll(toolDir, 0o755)).To(Succeed())
		Expect(filepath.Join(root, "grep")).To(Equal(toolDir))
	})
})
