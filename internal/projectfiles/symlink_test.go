package projectfiles

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RejectSymlinkPath", func() {
	It("accepts regular paths", func() {
		root := GinkgoT().TempDir()
		path := filepath.Join(root, "nested", "file.txt")

		Expect(RejectSymlinkPath(path)).To(Succeed())
	})

	It("rejects symlinked ancestor directories", func() {
		root := GinkgoT().TempDir()
		outside := GinkgoT().TempDir()
		targetDir := filepath.Join(outside, "nested")
		Expect(os.MkdirAll(targetDir, 0o755)).To(Succeed())

		linkPath := filepath.Join(root, "linked")
		if err := os.Symlink(outside, linkPath); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		err := RejectSymlinkPath(filepath.Join(linkPath, "nested", "file.txt"))

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("refuse to use symlink path component"))
		Expect(err.Error()).To(ContainSubstring(filepath.Base(linkPath)))
	})
})
