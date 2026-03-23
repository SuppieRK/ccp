package lifecycle

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("install script", func() {
	It("keeps the installed version in the success message", func() {
		body, err := os.ReadFile(filepath.Join("..", "..", "scripts", "install.sh"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(ContainSubstring(`echo "Installed $BIN_NAME $VERSION to $DST"`))
	})
})
