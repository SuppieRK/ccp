package version_test

import (
	vcmd "github.com/SuppieRK/cmdshape/internal/version"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Semantic", func() {
	DescribeTable("parsing strict semantic versions",
		func(raw string, expected vcmd.Semantic, ok bool) {
			actual, actualOK := vcmd.Parse(raw)

			Expect(actualOK).To(Equal(ok))
			Expect(actual).To(Equal(expected))
		},
		Entry("accepts zero version", "0.0.0", vcmd.Semantic{Major: 0, Minor: 0, Patch: 0}, true),
		Entry("accepts a normal release version", "1.2.3", vcmd.Semantic{Major: 1, Minor: 2, Patch: 3}, true),
		Entry("accepts large numeric components", "12.34.56", vcmd.Semantic{Major: 12, Minor: 34, Patch: 56}, true),
		Entry("rejects empty input", "", vcmd.Semantic{}, false),
		Entry("rejects missing patch", "1.2", vcmd.Semantic{}, false),
		Entry("rejects extra components", "1.2.3.4", vcmd.Semantic{}, false),
		Entry("rejects empty components", "1..3", vcmd.Semantic{}, false),
		Entry("rejects leading zeroes", "01.2.3", vcmd.Semantic{}, false),
		Entry("rejects v-prefixed versions", "v1.2.3", vcmd.Semantic{}, false),
		Entry("rejects prereleases", "1.2.3-rc.1", vcmd.Semantic{}, false),
		Entry("rejects build metadata", "1.2.3+meta", vcmd.Semantic{}, false),
		Entry("rejects whitespace", " 1.2.3 ", vcmd.Semantic{}, false),
		Entry("rejects non-numeric components", "1.two.3", vcmd.Semantic{}, false),
	)

	DescribeTable("formatting semantic versions",
		func(value vcmd.Semantic, expected string) {
			Expect(value.String()).To(Equal(expected))
		},
		Entry("formats zero version", vcmd.Semantic{}, "0.0.0"),
		Entry("formats populated version", vcmd.Semantic{Major: 1, Minor: 2, Patch: 3}, "1.2.3"),
	)

	DescribeTable("comparing versions",
		func(left vcmd.Semantic, right vcmd.Semantic, expected int, less bool, atLeast bool) {
			Expect(left.Compare(right)).To(Equal(expected))
			Expect(left.Less(right)).To(Equal(less))
			Expect(left.AtLeast(right)).To(Equal(atLeast))
		},
		Entry("major less", vcmd.Semantic{Major: 1}, vcmd.Semantic{Major: 2}, -1, true, false),
		Entry("minor less", vcmd.Semantic{Major: 1, Minor: 2}, vcmd.Semantic{Major: 1, Minor: 3}, -1, true, false),
		Entry("patch less", vcmd.Semantic{Major: 1, Minor: 2, Patch: 3}, vcmd.Semantic{Major: 1, Minor: 2, Patch: 4}, -1, true, false),
		Entry("equal", vcmd.Semantic{Major: 1, Minor: 2, Patch: 3}, vcmd.Semantic{Major: 1, Minor: 2, Patch: 3}, 0, false, true),
		Entry("patch greater", vcmd.Semantic{Major: 1, Minor: 2, Patch: 4}, vcmd.Semantic{Major: 1, Minor: 2, Patch: 3}, 1, false, true),
		Entry("minor greater", vcmd.Semantic{Major: 1, Minor: 3}, vcmd.Semantic{Major: 1, Minor: 2}, 1, false, true),
		Entry("major greater", vcmd.Semantic{Major: 2}, vcmd.Semantic{Major: 1}, 1, false, true),
	)

	Describe("Current", func() {
		It("parses the current build version when it is valid", func() {
			previous := vcmd.Version
			vcmd.Version = "1.2.3"
			DeferCleanup(func() { vcmd.Version = previous })

			actual, ok := vcmd.Current()

			Expect(ok).To(BeTrue())
			Expect(actual).To(Equal(vcmd.Semantic{Major: 1, Minor: 2, Patch: 3}))
		})

		It("reports invalid when the build version is not strict X.Y.Z", func() {
			previous := vcmd.Version
			vcmd.Version = "dev"
			DeferCleanup(func() { vcmd.Version = previous })

			actual, ok := vcmd.Current()

			Expect(ok).To(BeFalse())
			Expect(actual).To(Equal(vcmd.Semantic{}))
		})
	})
})
