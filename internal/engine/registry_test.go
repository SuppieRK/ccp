package engine

import (
	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/filters"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type registryFilter struct{}

func (registryFilter) PrepareCommand(command contracts.Command) (contracts.Command, error) {
	return command, nil
}

func (registryFilter) Dispatch(command contracts.Command) string {
	return command.Tool
}

func (registryFilter) OnStdout(string, contracts.Context) contracts.Action {
	return contracts.Action{Kind: contracts.ActionKeep}
}

func (registryFilter) OnStderr(string, contracts.Context) contracts.Action {
	return contracts.Action{Kind: contracts.ActionKeep}
}

func (registryFilter) OnStdoutExit(contracts.Context) contracts.Action {
	return contracts.Action{Kind: contracts.ActionKeep}
}

var _ = Describe("Registry", func() {
	It("returns a registered filter for the command tool", func() {
		registry := NewRegistry()
		expected := registryFilter{}
		registry.Register("demo", expected)

		resolved := registry.Resolve(contracts.Command{Tool: "demo"})

		Expect(resolved).To(Equal(expected))
	})

	It("falls back to passthrough when no filter matches", func() {
		registry := NewRegistry()

		resolved := registry.Resolve(contracts.Command{Tool: "unknown"})

		Expect(resolved).To(Equal(filters.Passthrough{}))
	})

	It("falls back to passthrough when the registry is nil", func() {
		var registry *Registry

		resolved := registry.Resolve(contracts.Command{Tool: "unknown"})

		Expect(resolved).To(Equal(filters.Passthrough{}))
	})

	It("resolves only explicitly registered filters", func() {
		registry := NewRegistry()
		expected := registryFilter{}
		registry.Register("python", expected)

		resolved := registry.Resolve(contracts.Command{
			Tool: "python",
			Args: []string{"python", "-m", "pytest", "-q"},
		})

		Expect(resolved).To(Equal(expected))
	})
})
