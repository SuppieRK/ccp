package engine

import (
	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/filters"
	yamlfilters "go-command-compression-proxy/internal/filters/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type registryContext struct {
	args     []string
	stdout   []string
	exitCode int
}

func (c registryContext) Args() []string {
	return c.args
}

func (c registryContext) BufferedLines(stream contracts.Stream) []string {
	if stream == contracts.StreamStdout || stream == contracts.StreamCombined {
		return c.stdout
	}
	return nil
}

func (c registryContext) ExitCode() int {
	return c.exitCode
}

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

	It("returns an isolated YAML filter instance for each resolution", func() {
		registry := NewRegistry()
		filter, err := yamlfilters.NewFilter(&yamlfilters.FilterDefinition{
			Version: 1,
			Filter:  "ls",
			Cases: []yamlfilters.CaseClause{{
				ID: "long",
				Variables: []yamlfilters.Variable{
					{Name: "dirs", Type: "number"},
					{Name: "files", Type: "number"},
				},
				CompressOutput: &yamlfilters.OutputShape{
					Combined: &yamlfilters.OutputScope{
						Lines: &yamlfilters.OutputLines{
							Replace: []yamlfilters.ReplaceRule{
								{
									Regex: `^d\S*(?:\s+\S+){7}\s+(.+)$`,
									To:    new("$1/"),
									OnMatch: []yamlfilters.MatchAction{{
										Variable:  "dirs",
										Increment: new(1),
									}},
								},
								{
									Regex: `^[\-l]\S*(?:\s+\S+){3}\s+(\S+)(?:\s+\S+){3}\s+(.+)$`,
									To:    new("$2  $1"),
									OnMatch: []yamlfilters.MatchAction{{
										Variable:  "files",
										Increment: new(1),
									}},
								},
							},
						},
					},
				},
				Finally: &yamlfilters.OnExit{Print: "{{dirs}} dirs, {{files}} files"},
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		registry.Register("ls", filter)

		command := contracts.Command{Tool: "ls", Args: []string{"ls", "-la"}}

		first := registry.Resolve(command)
		firstDir := first.OnStdout("drwxrwxrwx 1 suppie suppie 4.0K Feb 26 11:50 docs\n", registryContext{args: command.Args})
		firstFile := first.OnStdout("-rwxrwxrwx 1 suppie suppie 59 Feb 26 11:50 README.md\n", registryContext{args: command.Args})
		firstExit := first.OnStdoutExit(registryContext{
			args:   command.Args,
			stdout: []string{firstDir.Output, firstFile.Output},
		})
		Expect(firstExit.Output).To(Equal("docs/\nREADME.md  59\n1 dirs, 1 files\n"))

		second := registry.Resolve(command)
		secondFile := second.OnStdout("-rwxrwxrwx 1 suppie suppie 59 Feb 26 11:50 README.md\n", registryContext{args: command.Args})
		secondExit := second.OnStdoutExit(registryContext{
			args:   command.Args,
			stdout: []string{secondFile.Output},
		})
		Expect(secondExit.Output).To(Equal("README.md  59\n0 dirs, 1 files\n"))
	})
})
