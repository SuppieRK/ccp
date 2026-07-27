package engine

import (
	"github.com/SuppieRK/cmdshape/internal/contracts"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Engine.StartResolved", func() {
	It("uses the supplied filter without resolving or cloning it again", func() {
		filter := &resolvedFilter{}
		registry := NewRegistry()
		registry.Register("demo", filter)

		state := NewEngine(registry).StartResolved(contracts.Command{
			Tool: "demo",
			Args: []string{"demo"},
		}, filter)

		Expect(state.Stdout("kept\n")).To(BeEmpty())
		Expect(filter.stdoutCalls).To(Equal(1))
	})
})

type resolvedFilter struct {
	stdoutCalls int
}

func (f *resolvedFilter) PrepareCommand(command contracts.Command) (contracts.Command, error) {
	return command, nil
}

func (f *resolvedFilter) Dispatch(contracts.Command) string {
	return "demo"
}

func (f *resolvedFilter) OnStdout(string, contracts.Context) contracts.Action {
	f.stdoutCalls++
	return contracts.Action{Kind: contracts.ActionKeep}
}

func (f *resolvedFilter) OnStderr(string, contracts.Context) contracts.Action {
	return contracts.Action{Kind: contracts.ActionKeep}
}

func (f *resolvedFilter) OnStdoutExit(contracts.Context) contracts.Action {
	return contracts.Action{Kind: contracts.ActionKeep}
}
