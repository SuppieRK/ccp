package agents

import (
	"os"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("managed family support helpers", func() {
	ginkgo.It("reports noop when no managed change needs to be applied", func() {
		res, err := applyManagedFileChange(filepath.Join(ginkgo.GinkgoT().TempDir(), "settings.json"), "", false, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(InstallResult{Noop: 1}))
	})

	ginkgo.It("reports noop when removing an already-missing managed file", func() {
		res, err := applyManagedFileChange(filepath.Join(ginkgo.GinkgoT().TempDir(), "missing.json"), "", true, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(InstallResult{Noop: 1}))
	})

	ginkgo.It("refuses to rewrite managed files through symlinked paths", func() {
		tmpDir := ginkgo.GinkgoT().TempDir()
		outsideDir := filepath.Join(tmpDir, "outside")
		Expect(os.MkdirAll(outsideDir, 0o755)).To(Succeed())
		outsideFile := filepath.Join(outsideDir, "settings.json")
		Expect(os.WriteFile(outsideFile, []byte("keep me\n"), 0o644)).To(Succeed())

		linkDir := filepath.Join(tmpDir, ".agent")
		if err := os.Symlink(outsideDir, linkDir); err != nil {
			ginkgo.Skip("symlink creation unavailable: " + err.Error())
		}

		res, err := applyManagedFileChange(filepath.Join(linkDir, "settings.json"), "overwrite\n", true, false)
		Expect(err).To(HaveOccurred())
		Expect(res).To(Equal(InstallResult{}))

		body, readErr := os.ReadFile(outsideFile)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("keep me\n"))
	})

	ginkgo.It("refuses to remove managed files through symlinked paths", func() {
		tmpDir := ginkgo.GinkgoT().TempDir()
		outsideDir := filepath.Join(tmpDir, "outside")
		Expect(os.MkdirAll(outsideDir, 0o755)).To(Succeed())
		outsideFile := filepath.Join(outsideDir, "cmdshape.md")
		Expect(os.WriteFile(outsideFile, []byte("keep me\n"), 0o644)).To(Succeed())

		linkDir := filepath.Join(tmpDir, ".rules")
		if err := os.Symlink(outsideDir, linkDir); err != nil {
			ginkgo.Skip("symlink creation unavailable: " + err.Error())
		}

		removed, err := removeFileIfExists(filepath.Join(linkDir, "cmdshape.md"))
		Expect(err).To(HaveOccurred())
		Expect(removed).To(BeFalse())

		body, readErr := os.ReadFile(outsideFile)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("keep me\n"))
	})
})
