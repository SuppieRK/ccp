package agents

import (
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("adapter catalog", func() {
	ginkgo.Describe("NewBuiltInAdapters", func() {
		ginkgo.It("includes every built-in adapter from the catalog", func() {
			adapters, err := NewBuiltInAdapters()
			Expect(err).NotTo(HaveOccurred())

			for _, spec := range BuiltInAdapterCatalog() {
				id := string(spec.ID)
				Expect(adapters).To(HaveKey(id))
			}
		})
	})

	ginkgo.Describe("adaptersFromCatalog", func() {
		ginkgo.It("rejects duplicate canonical ids", func() {
			_, err := adaptersFromCatalog([]BuiltInAdapterSpec{
				{ID: AgentCodex, New: func() Adapter { return NewManagedContextAdapter(codexContextSpec) }},
				{ID: AgentCodex, New: func() Adapter { return NewManagedContextAdapter(codexContextSpec) }},
			})

			Expect(err).To(HaveOccurred())
		})

		ginkgo.It("rejects adapters whose runtime id does not match the catalog id", func() {
			_, err := adaptersFromCatalog([]BuiltInAdapterSpec{
				{ID: AgentCodex, New: func() Adapter { return ClaudeAdapter{} }},
			})

			Expect(err).To(HaveOccurred())
		})
	})
})
