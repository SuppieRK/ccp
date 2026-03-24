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

	It("requires explicit versions to match exact semantic version format", func() {
		body, err := os.ReadFile(filepath.Join("..", "..", "scripts", "install.sh"))
		Expect(err).NotTo(HaveOccurred())

		script := string(body)
		Expect(script).To(ContainSubstring(`validate_release_version()`))
		Expect(script).To(ContainSubstring(`release version must be exact semantic version (X.Y.Z): ${VERSION}`))
		Expect(script).NotTo(ContainSubstring(`${ver#v}`))
		Expect(script).NotTo(ContainSubstring(`${ver%%-*}`))
		Expect(script).NotTo(ContainSubstring(`${ver%%+*}`))
	})

	It("keeps the release workflow pinned to exact semantic version tags", func() {
		body, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release-distribution.yml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(ContainSubstring(`[[ ! "${TAG}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]`))
		Expect(string(body)).To(ContainSubstring(`release tag must match X.Y.Z exactly: ${TAG}`))
	})
})
