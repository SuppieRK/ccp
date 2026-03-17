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

	It("materializes the shipped filter files", func() {
		Expect(MaterializeShipped(dst)).To(Succeed())

		_, err := os.Stat(filepath.Join(dst, ".mappings.yaml"))
		Expect(err).NotTo(HaveOccurred())

		_, err = os.Stat(filepath.Join(dst, "npm.yaml"))
		Expect(err).NotTo(HaveOccurred())

		_, err = os.Stat(filepath.Join(dst, "git.yaml"))
		Expect(err).NotTo(HaveOccurred())
	})
})
