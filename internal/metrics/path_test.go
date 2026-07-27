package metrics

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ProjectPath", func() {
	It("returns the canonical workspace metrics path", func() {
		Expect(ProjectPath(filepath.Join("workspace", "repo"))).To(Equal(
			filepath.Join("workspace", "repo", ".cmdshape", "gain.db"),
		))
	})

	It("returns an empty path for an empty workspace root", func() {
		Expect(ProjectPath("")).To(BeEmpty())
	})
})
