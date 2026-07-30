package lifecycle

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("embedded filter prompt template", func() {
	It("uses one filter ID placeholder form", func() {
		Expect(embeddedFilterPrompt).To(ContainSubstring("{{FILTER_ID}}"))
		Expect(embeddedFilterPrompt).ToNot(ContainSubstring("<filter-id>"))
	})
})
