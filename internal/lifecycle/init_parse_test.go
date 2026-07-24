package lifecycle

import (
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/SuppieRK/cmdshape/internal/lifecycle/agents"
)

var _ = Describe("init argument parsing", func() {
	DescribeTable("normalizes tool lists",
		func(input string, expected []string) {
			Expect(parseTools(input)).To(Equal(expected))
		},
		Entry("ignoring empty input", "", []string(nil)),
		Entry("sorting and deduplicating mixed-case input", " Git , go ,git,  ,DOCKER,go ", []string{"docker", "git", "go"}),
		Entry("normalizing aliases", " costrict , roocode ", []string{"roocode"}),
	)

	It("keeps parseTools output stable", func() {
		got := parseTools("git,go,docker")
		Expect(slices.IsSorted(got)).To(BeTrue())
	})

	Context("when init arguments are invalid", func() {
		BeforeEach(func() {
			newLifecycleWorkspaceSpec()
		})

		It("requires tools when none are provided or detected", func() {
			err := RunInit(nil)
			Expect(err).To(MatchError(ContainSubstring("no tools detected")))
		})

		It("rejects unsupported tools", func() {
			err := RunInit([]string{"--tools", "unknown"})
			Expect(err).To(MatchError(ContainSubstring("unsupported tool")))
		})
	})

	It("returns explicit tools without requiring repository detection", func() {
		tools, err := resolveInitTools(" costrict , codex ", map[string]agents.Adapter{
			"codex":   &fakeInstallAdapter{},
			"roocode": &fakeInstallAdapter{},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(tools).To(Equal([]string{"codex", "roocode"}))
	})
})
