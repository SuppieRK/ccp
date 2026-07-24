package projectfiles

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const gitignorePathName = ".gitignore"

var _ = Describe("EnsureNestedCmdshapeGitignore", func() {
	It("creates the nested cmdshape-owned gitignore with canonical contents", func() {
		root := GinkgoT().TempDir()

		Expect(EnsureNestedCmdshapeGitignore(root)).To(Succeed())

		body, err := os.ReadFile(filepath.Join(root, ".cmdshape", gitignorePathName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("gain.db\n.gitignore\n"))
	})

	It("overwrites user edits with canonical contents", func() {
		root := GinkgoT().TempDir()
		path := filepath.Join(root, ".cmdshape", gitignorePathName)
		Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
		Expect(os.WriteFile(path, []byte("custom\n!filters/\n"), 0o644)).To(Succeed())

		Expect(EnsureNestedCmdshapeGitignore(root)).To(Succeed())
		Expect(EnsureNestedCmdshapeGitignore(root)).To(Succeed())

		body, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("gain.db\n.gitignore\n"))
	})

	It("treats a blank root as a noop", func() {
		Expect(EnsureNestedCmdshapeGitignore("  ")).To(Succeed())
	})

	It("returns an error when .cmdshape is a file", func() {
		root := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(root, ".cmdshape"), []byte("not a directory"), 0o644)).To(Succeed())

		err := EnsureNestedCmdshapeGitignore(root)

		Expect(err).To(HaveOccurred())
	})

	It("refuses to write through a symlinked .cmdshape directory", func() {
		root := GinkgoT().TempDir()
		outside := GinkgoT().TempDir()
		if err := os.Symlink(outside, filepath.Join(root, ".cmdshape")); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		err := EnsureNestedCmdshapeGitignore(root)

		Expect(err).To(HaveOccurred())
		_, statErr := os.Stat(filepath.Join(outside, gitignorePathName))
		Expect(statErr).To(MatchError(os.ErrNotExist))
	})

	It("refuses to overwrite a symlinked nested gitignore", func() {
		root := GinkgoT().TempDir()
		outside := filepath.Join(GinkgoT().TempDir(), "outside-gitignore")
		path := filepath.Join(root, ".cmdshape", gitignorePathName)
		Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
		Expect(os.WriteFile(outside, []byte("keep\n"), 0o644)).To(Succeed())
		if err := os.Symlink(outside, path); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		err := EnsureNestedCmdshapeGitignore(root)

		Expect(err).To(HaveOccurred())
		body, readErr := os.ReadFile(outside)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("keep\n"))
	})
})

var _ = Describe("RemoveProductRootGitignoreEntries", func() {
	DescribeTable("removing only exact legacy entries",
		func(initial string, expected string) {
			root := GinkgoT().TempDir()
			path := filepath.Join(root, gitignorePathName)
			Expect(os.WriteFile(path, []byte(initial), 0o644)).To(Succeed())

			Expect(RemoveProductRootGitignoreEntries(root)).To(Succeed())
			Expect(RemoveProductRootGitignoreEntries(root)).To(Succeed())

			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal(expected))
		},
		Entry("removes dot cmdshape lines", "node_modules/\n.cmdshape\n.cmdshape/\n", "node_modules/\n"),
		Entry("trims whitespace before matching", "  .cmdshape  \n\t.cmdshape/\t\nkeep\n", "keep\n"),
		Entry("preserves comments and non-legacy patterns", "# .cmdshape\nfoo.cmdshape\n.cmdshape/**\n!.cmdshape/filters/\n", "# .cmdshape\nfoo.cmdshape\n.cmdshape/**\n!.cmdshape/filters/\n"),
		Entry("preserves CRLF on kept lines", "node_modules/\r\n.cmdshape\r\nkeep\r\n", "node_modules/\r\nkeep\r\n"),
		Entry("handles a final line without newline", "keep\n.cmdshape", "keep\n"),
		Entry("can remove all lines", ".cmdshape\n.cmdshape/\n", ""),
	)

	It("treats a missing gitignore as a noop", func() {
		root := GinkgoT().TempDir()

		Expect(RemoveProductRootGitignoreEntries(root)).To(Succeed())

		_, err := os.Stat(filepath.Join(root, gitignorePathName))
		Expect(err).To(MatchError(os.ErrNotExist))
	})

	It("treats a blank root as a noop", func() {
		Expect(RemoveProductRootGitignoreEntries("  ")).To(Succeed())
	})

	It("returns an error when .gitignore is a directory", func() {
		root := GinkgoT().TempDir()
		Expect(os.Mkdir(filepath.Join(root, gitignorePathName), 0o755)).To(Succeed())

		err := RemoveProductRootGitignoreEntries(root)

		Expect(err).To(HaveOccurred())
	})

	It("refuses to update a symlinked root gitignore", func() {
		root := GinkgoT().TempDir()
		outside := filepath.Join(GinkgoT().TempDir(), "outside-gitignore")
		path := filepath.Join(root, gitignorePathName)
		Expect(os.WriteFile(outside, []byte(".cmdshape\n"), 0o644)).To(Succeed())
		if err := os.Symlink(outside, path); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		err := RemoveProductRootGitignoreEntries(root)

		Expect(err).To(HaveOccurred())
		body, readErr := os.ReadFile(outside)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal(".cmdshape\n"))
	})
})
