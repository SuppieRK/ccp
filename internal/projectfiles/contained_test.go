package projectfiles

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("contained project files", func() {
	It("creates and opens a regular file beneath the root", func() {
		root := GinkgoT().TempDir()
		path := filepath.Join(root, ".ccp", "gain.db")

		file, err := OpenFileBeneath(root, path, os.O_RDWR|os.O_CREATE, 0o600)
		Expect(err).NotTo(HaveOccurred())
		Expect(file.Close()).To(Succeed())

		Expect(path).To(BeAnExistingFile())
	})

	It("accepts a path expressed through the same symlinked root alias", func() {
		realRoot := GinkgoT().TempDir()
		aliasRoot := filepath.Join(GinkgoT().TempDir(), "project")
		if err := os.Symlink(realRoot, aliasRoot); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}
		path := filepath.Join(aliasRoot, ".ccp", "gain.db")

		file, err := OpenFileBeneath(aliasRoot, path, os.O_RDWR|os.O_CREATE, 0o600)

		Expect(err).NotTo(HaveOccurred())
		Expect(file.Close()).To(Succeed())
		Expect(filepath.Join(realRoot, ".ccp", "gain.db")).To(BeAnExistingFile())
	})

	It("rejects paths outside the root", func() {
		root := GinkgoT().TempDir()
		outside := filepath.Join(GinkgoT().TempDir(), "gain.db")

		_, err := OpenFileBeneath(root, outside, os.O_RDWR|os.O_CREATE, 0o600)

		Expect(err).To(MatchError(ContainSubstring("outside contained root")))
		Expect(outside).NotTo(BeAnExistingFile())
	})

	It("rejects canonical paths outside the root", func() {
		root := GinkgoT().TempDir()
		outside := filepath.Join(GinkgoT().TempDir(), "gain.db")

		_, err := CanonicalPathBeneath(root, outside)

		Expect(err).To(MatchError(ContainSubstring("outside contained root")))
	})

	It("reports a missing regular file during validation", func() {
		root := GinkgoT().TempDir()
		path := filepath.Join(root, ".ccp", "missing.db")

		err := ValidateRegularFileBeneath(root, path)

		Expect(err).To(HaveOccurred())
	})

	It("canonicalizes and validates an existing regular file beneath the root", func() {
		root := GinkgoT().TempDir()
		path := filepath.Join(root, ".ccp", "gain.db")
		Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
		Expect(os.WriteFile(path, []byte("metrics"), 0o600)).To(Succeed())

		canonical, err := CanonicalPathBeneath(root, path)

		Expect(err).NotTo(HaveOccurred())
		expected, err := filepath.EvalSymlinks(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(canonical).To(Equal(expected))
		Expect(ValidateRegularFileBeneath(root, path)).To(Succeed())
	})

	It("rejects a missing contained root", func() {
		root := filepath.Join(GinkgoT().TempDir(), "missing")
		path := filepath.Join(root, ".ccp", "gain.db")

		_, err := CanonicalPathBeneath(root, path)

		Expect(err).To(HaveOccurred())
	})

	It("rejects symlinked parent directories", func() {
		root := GinkgoT().TempDir()
		outside := GinkgoT().TempDir()
		link := filepath.Join(root, ".ccp")
		if err := os.Symlink(outside, link); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		_, err := OpenFileBeneath(root, filepath.Join(link, "gain.db"), os.O_RDWR|os.O_CREATE, 0o600)

		Expect(err).To(HaveOccurred())
		Expect(filepath.Join(outside, "gain.db")).NotTo(BeAnExistingFile())
	})

	It("rejects symlinked final components", func() {
		root := GinkgoT().TempDir()
		ccpDir := filepath.Join(root, ".ccp")
		Expect(os.Mkdir(ccpDir, 0o755)).To(Succeed())
		outside := filepath.Join(GinkgoT().TempDir(), "outside.db")
		Expect(os.WriteFile(outside, []byte("keep"), 0o600)).To(Succeed())
		link := filepath.Join(ccpDir, "gain.db")
		if err := os.Symlink(outside, link); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		_, err := OpenFileBeneath(root, link, os.O_RDWR|os.O_CREATE, 0o600)

		Expect(err).To(HaveOccurred())
		body, readErr := os.ReadFile(outside)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(body).To(Equal([]byte("keep")))
	})

	It("rejects hard-linked final components", func() {
		root := GinkgoT().TempDir()
		ccpDir := filepath.Join(root, ".ccp")
		Expect(os.Mkdir(ccpDir, 0o755)).To(Succeed())
		outside := filepath.Join(GinkgoT().TempDir(), "outside.db")
		Expect(os.WriteFile(outside, []byte("keep"), 0o600)).To(Succeed())
		link := filepath.Join(ccpDir, "gain.db")
		Expect(os.Link(outside, link)).To(Succeed())

		_, err := OpenFileBeneath(root, link, os.O_RDWR, 0o600)

		Expect(err).To(MatchError(ContainSubstring("hard-linked")))
	})

	It("stays on the opened parent when its pathname is swapped before final open", func() {
		root := GinkgoT().TempDir()
		ccpDir := filepath.Join(root, ".ccp")
		heldDir := filepath.Join(root, ".ccp-held")
		Expect(os.Mkdir(ccpDir, 0o755)).To(Succeed())
		outside := GinkgoT().TempDir()
		beforeContainedFinalOpen = func() {
			beforeContainedFinalOpen = func() {}
			Expect(os.Rename(ccpDir, heldDir)).To(Succeed())
			if err := os.Symlink(outside, ccpDir); err != nil {
				Skip("symlink creation unavailable: " + err.Error())
			}
		}
		DeferCleanup(func() {
			beforeContainedFinalOpen = func() {}
		})

		file, err := OpenFileBeneath(root, filepath.Join(ccpDir, "gain.db"), os.O_RDWR|os.O_CREATE, 0o600)

		Expect(err).NotTo(HaveOccurred())
		Expect(file.Close()).To(Succeed())
		Expect(filepath.Join(heldDir, "gain.db")).To(BeAnExistingFile())
		Expect(filepath.Join(outside, "gain.db")).NotTo(BeAnExistingFile())
	})

	It("rejects non-regular final targets", func() {
		root := GinkgoT().TempDir()
		path := filepath.Join(root, ".ccp", "gain.db")
		Expect(os.MkdirAll(path, 0o755)).To(Succeed())

		_, err := OpenFileBeneath(root, path, os.O_RDONLY, 0)

		Expect(err).To(HaveOccurred())
	})

	It("atomically writes beneath the root without following links", func() {
		root := GinkgoT().TempDir()
		path := filepath.Join(root, ".ccp", ".gitignore")

		Expect(AtomicWriteFileBeneath(root, path, []byte("gain.db\n"), 0o644)).To(Succeed())
		Expect(AtomicWriteFileBeneath(root, path, []byte("gain.db\n.gitignore\n"), 0o600)).To(Succeed())

		body, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(Equal([]byte("gain.db\n.gitignore\n")))
		if os.PathSeparator == '/' {
			info, statErr := os.Stat(path)
			Expect(statErr).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o644)))
		}
	})
})
