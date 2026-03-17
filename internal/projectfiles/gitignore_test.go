package projectfiles

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const gitignorePathName = ".gitignore"

var _ = Describe("EnsureGitignoreEntry", func() {
	DescribeTable("updating gitignore content",
		func(initial string, expected string) {
			root := GinkgoT().TempDir()
			path := filepath.Join(root, gitignorePathName)
			Expect(os.WriteFile(path, []byte(initial), 0o644)).To(Succeed())

			Expect(EnsureGitignoreEntry(root, ".ccp")).To(Succeed())

			b, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(expected))
		},
		Entry("appends", "node_modules\n", "node_modules\n.ccp\n"),
		Entry("avoids duplicates", ".ccp\n", ".ccp\n"),
		Entry("appends without trailing newline", "node_modules", "node_modules\n.ccp\n"),
	)

	Context("when the gitignore file is missing", func() {
		var root string

		BeforeEach(func() {
			root = GinkgoT().TempDir()
		})

		It("leaves it absent", func() {
			Expect(EnsureGitignoreEntry(root, ".ccp")).To(Succeed())

			_, err := os.Stat(filepath.Join(root, gitignorePathName))
			Expect(err).To(MatchError(os.ErrNotExist))
		})
	})

	Context("when given blank inputs", func() {
		var (
			root string
			path string
		)

		BeforeEach(func() {
			root = GinkgoT().TempDir()
			path = filepath.Join(root, gitignorePathName)
			Expect(os.WriteFile(path, []byte("node_modules\n"), 0o644)).To(Succeed())
		})

		It("treats a blank root as a noop", func() {
			Expect(EnsureGitignoreEntry("   ", ".ccp")).To(Succeed())

			b, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal("node_modules\n"))
		})

		It("treats a blank entry as a noop", func() {
			Expect(EnsureGitignoreEntry(root, "   ")).To(Succeed())

			b, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal("node_modules\n"))
		})
	})
})
