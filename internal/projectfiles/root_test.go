package projectfiles

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ResolveProjectRoot", func() {
	It("finds the nearest enclosing Git directory", func() {
		root := GinkgoT().TempDir()
		nested := filepath.Join(root, "src", "feature")
		Expect(os.MkdirAll(filepath.Join(root, ".git"), 0o755)).To(Succeed())
		Expect(os.MkdirAll(nested, 0o755)).To(Succeed())

		resolved, err := ResolveProjectRoot(nested)

		Expect(err).NotTo(HaveOccurred())
		Expect(resolved).To(Equal(canonicalTestPath(root)))
	})

	It("recognizes Git worktree files", func() {
		root := GinkgoT().TempDir()
		nested := filepath.Join(root, "src")
		Expect(os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: elsewhere\n"), 0o644)).To(Succeed())
		Expect(os.MkdirAll(nested, 0o755)).To(Succeed())

		resolved, err := ResolveProjectRoot(nested)

		Expect(err).NotTo(HaveOccurred())
		Expect(resolved).To(Equal(canonicalTestPath(root)))
	})

	It("prefers the nearest nested repository", func() {
		root := GinkgoT().TempDir()
		nestedRoot := filepath.Join(root, "vendor", "nested")
		work := filepath.Join(nestedRoot, "src")
		Expect(os.MkdirAll(filepath.Join(root, ".git"), 0o755)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(nestedRoot, ".git"), 0o755)).To(Succeed())
		Expect(os.MkdirAll(work, 0o755)).To(Succeed())

		resolved, err := ResolveProjectRoot(work)

		Expect(err).NotTo(HaveOccurred())
		Expect(resolved).To(Equal(canonicalTestPath(nestedRoot)))
	})

	It("uses the starting directory outside Git repositories", func() {
		root := GinkgoT().TempDir()
		nested := filepath.Join(root, "src")
		Expect(os.MkdirAll(nested, 0o755)).To(Succeed())

		resolved, err := ResolveProjectRoot(nested)

		Expect(err).NotTo(HaveOccurred())
		Expect(resolved).To(Equal(canonicalTestPath(nested)))
	})

	It("does not treat a symlinked Git marker as a repository", func() {
		root := GinkgoT().TempDir()
		work := filepath.Join(root, "src")
		markerTarget := filepath.Join(GinkgoT().TempDir(), "git-marker")
		Expect(os.MkdirAll(work, 0o755)).To(Succeed())
		Expect(os.MkdirAll(markerTarget, 0o755)).To(Succeed())
		if err := os.Symlink(markerTarget, filepath.Join(root, ".git")); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		resolved, err := ResolveProjectRoot(work)

		Expect(err).NotTo(HaveOccurred())
		Expect(resolved).To(Equal(canonicalTestPath(work)))
	})
})

func canonicalTestPath(path string) string {
	canonical, err := filepath.EvalSymlinks(path)
	Expect(err).NotTo(HaveOccurred())
	return canonical
}
