package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SuppieRK/cmdshape/internal/cli"
	"github.com/SuppieRK/cmdshape/internal/lifecycle/agents"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("generated CLI facts", func() {
	It("uses the runtime command and integration inventories", func() {
		Expect(cli.LifecycleCommands()).To(ContainElements("capture", "filter", "history", "migrate", "recovery", "upgrade", "verify"))
		adapters, err := agents.NewBuiltInAdapters()
		Expect(err).NotTo(HaveOccurred())
		Expect(agents.SupportedTools(adapters)).NotTo(BeEmpty())
	})

	It("keeps the checked-in generated document present", func() {
		root := filepath.Join("..", "..", "docs", "generated", "CLI_FACTS.md")
		checkedIn, err := os.ReadFile(root)
		Expect(err).NotTo(HaveOccurred())
		rendered, err := renderGeneratedFacts()
		Expect(err).NotTo(HaveOccurred())
		Expect(checkedIn).To(Equal(rendered), "run `go run ./cmd/cmdshape-docgen`")
	})

	It("keeps the generated inventory linked from the README", func() {
		readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(readme)).To(ContainSubstring("[docs/generated/CLI_FACTS.md](docs/generated/CLI_FACTS.md)"))
	})
})

func TestDocgen(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "cmdshape Docgen Suite")
}
