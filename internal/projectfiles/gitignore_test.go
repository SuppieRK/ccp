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
