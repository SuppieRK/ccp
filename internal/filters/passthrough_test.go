package filters

import (
	"path/filepath"

	"github.com/SuppieRK/cmdshape/internal/contracts"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type passthroughContext struct{}

func (passthroughContext) Args() []string {
	return nil
}

func (passthroughContext) BufferedLines(contracts.Stream) []string {
	return nil
}

func (passthroughContext) ExitCode() int {
	return 0
}

var _ = Describe("Passthrough", func() {
	var (
		filter  Passthrough
		context contracts.Context
		command contracts.Command
	)

	BeforeEach(func() {
		filter = Passthrough{}
		context = passthroughContext{}
		command = contracts.Command{Tool: "ls", Args: []string{"ls"}}
	})

	It("leaves the command unchanged during preparation", func() {
		prepared, err := filter.PrepareCommand(command)

		Expect(err).NotTo(HaveOccurred())
		Expect(prepared).To(Equal(command))
	})

	It("dispatches using the command tool", func() {
		Expect(filter.Dispatch(command)).To(Equal("ls"))
	})

	It("emits stdout lines unchanged", func() {
		Expect(filter.OnStdout("out\n", context)).To(Equal(contracts.Action{Kind: contracts.ActionEmit}))
	})

	It("emits stderr lines unchanged", func() {
		Expect(filter.OnStderr("err\n", context)).To(Equal(contracts.Action{Kind: contracts.ActionEmit}))
	})

	It("emits the stdout exit action unchanged", func() {
		filter := Passthrough{}
		Expect(filter.OnStdoutExit(context)).To(Equal(contracts.Action{Kind: contracts.ActionEmit}))
	})
})

var _ = Describe("filter sources", func() {
	DescribeTable("building source directories",
		func(source func(string) FilterSource, root string, expected FilterSource) {
			Expect(source(root)).To(Equal(expected))
		},
		Entry("repository source", RepositorySource, "/repo", FilterSource{
			Kind:      SourceRepository,
			Directory: filepath.Join("/repo", "filters"),
		}),
		Entry("project source", ProjectSource, "/repo", FilterSource{
			Kind:      SourceProject,
			Directory: filepath.Join("/repo", ".cmdshape", "filters"),
		}),
		Entry("home source", HomeSource, "/home/user", FilterSource{
			Kind:      SourceHome,
			Directory: filepath.Join("/home/user", ".config", "cmdshape", "filters"),
		}),
	)
})
