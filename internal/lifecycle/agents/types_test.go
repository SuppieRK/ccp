package agents

import (
	"os"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("agent type helpers", func() {
	ginkgo.Describe("SupportedTools", func() {
		ginkgo.It("returns tool ids in sorted order", func() {
			adapters := map[string]Adapter{
				"zeta":  stubAdapter{id: "zeta"},
				"alpha": stubAdapter{id: "alpha"},
				"beta":  stubAdapter{id: "beta"},
			}

			Expect(SupportedTools(adapters)).To(Equal([]string{"alpha", "beta", "zeta"}))
		})
	})

	ginkgo.Describe("ValidateSelectedTools", func() {
		var adapters map[string]Adapter

		ginkgo.BeforeEach(func() {
			adapters = map[string]Adapter{"alpha": stubAdapter{id: "alpha"}}
		})

		ginkgo.It("accepts supported tools", func() {
			Expect(ValidateSelectedTools([]string{"alpha"}, adapters)).To(Succeed())
		})

		ginkgo.It("rejects unsupported tools", func() {
			Expect(ValidateSelectedTools([]string{"beta"}, adapters)).To(HaveOccurred())
		})
	})

	ginkgo.Describe("DetectTools", func() {
		var (
			root string
		)

		ginkgo.BeforeEach(func() {
			root = ginkgo.GinkgoT().TempDir()
		})

		ginkgo.It("returns only detected adapter roots", func() {
			Expect(os.Mkdir(filepath.Join(root, "alpha-root"), 0o755)).To(Succeed())

			adapters := map[string]Adapter{
				"alpha": stubAdapter{id: "alpha", detectDir: "alpha-root"},
				"beta":  stubAdapter{id: "beta"},
			}

			Expect(DetectTools(root, adapters)).To(Equal([]string{"alpha"}))
		})

		ginkgo.It("ignores non-directory collisions", func() {
			filePath := filepath.Join(root, "alpha-root")
			Expect(os.WriteFile(filePath, []byte("not-a-dir"), 0o644)).To(Succeed())
			Expect(os.Mkdir(filepath.Join(root, "beta-root"), 0o755)).To(Succeed())

			adapters := map[string]Adapter{
				"alpha": stubAdapter{id: "alpha", detectDir: "alpha-root"},
				"beta":  stubAdapter{id: "beta", detectDir: "beta-root"},
			}

			Expect(DetectTools(root, adapters)).To(Equal([]string{"beta"}))
		})
	})

	ginkgo.Describe("NewBuiltInAdapters", func() {
		ginkgo.It("contains every built-in catalog id with a matching adapter id", func() {
			adapters, err := NewBuiltInAdapters()
			Expect(err).NotTo(HaveOccurred())

			for _, spec := range BuiltInAdapterCatalog() {
				id := string(spec.ID)
				adapter, ok := adapters[id]
				Expect(ok).To(BeTrue(), "expected adapter %q", id)
				Expect(adapter.ID()).To(Equal(id))
			}
		})
	})
})
