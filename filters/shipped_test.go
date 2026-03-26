package filters

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MaterializeShipped", func() {
	var dst string

	BeforeEach(func() {
		dst = filepath.Join(GinkgoT().TempDir(), "filters")
	})

	Context("when the destination directory is writable", func() {
		It("materializes the shipped filter files", func() {
			Expect(MaterializeShipped(dst)).To(Succeed())

			_, err := os.Stat(filepath.Join(dst, ".mappings.yaml"))
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(filepath.Join(dst, "npm.yaml"))
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(filepath.Join(dst, "git.yaml"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("overwrites existing shipped files on subsequent runs", func() {
			Expect(MaterializeShipped(dst)).To(Succeed())

			filterPath := filepath.Join(dst, "git.yaml")
			Expect(os.WriteFile(filterPath, []byte("mutated"), 0o644)).To(Succeed())

			Expect(MaterializeShipped(dst)).To(Succeed())

			body, err := os.ReadFile(filterPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).NotTo(Equal("mutated"))
		})
	})

	Context("when the destination path is blocked by a file", func() {
		It("returns an error", func() {
			blocked := filepath.Join(GinkgoT().TempDir(), "blocked")
			Expect(os.WriteFile(blocked, []byte("no-dir"), 0o644)).To(Succeed())

			err := MaterializeShipped(blocked)

			Expect(err).To(HaveOccurred())
		})
	})
})
