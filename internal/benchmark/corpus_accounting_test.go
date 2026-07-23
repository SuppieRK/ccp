package benchmark

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	filteryaml "go-command-compression-proxy/internal/filters/yaml"
	"go-command-compression-proxy/internal/replay"

	"gopkg.in/yaml.v3"
)

type corpusMappings struct {
	Version int               `yaml:"version"`
	Map     map[string]string `yaml:"map"`
}

var _ = Describe("shipped corpus accounting", func() {
	It("exercises every filter case and command mapping with complete assertions", func() {
		root := filteryaml.ProjectRootFromSource()
		fixturesRoot := filepath.Join(root, "testdata", "benchmarks")
		filterPaths, err := filepath.Glob(filepath.Join(root, "filters", "*.yaml"))
		Expect(err).NotTo(HaveOccurred())
		fixturePaths, err := filepath.Glob(filepath.Join(fixturesRoot, "*", "*", replay.CommandFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(fixturePaths).NotTo(BeEmpty())

		dispatches := make(map[string]string, len(fixturePaths))
		invoked := make(map[string]struct{}, len(fixturePaths))
		for _, commandPath := range fixturePaths {
			dir := filepath.Dir(commandPath)
			spec, readErr := replay.ReadCommand(commandPath)
			Expect(readErr).NotTo(HaveOccurred(), commandPath)
			Expect(spec.ExitCodeAsserted).To(BeTrue(), commandPath)
			invoked[filepath.Base(spec.Argv[0])] = struct{}{}

			for _, name := range []string{
				replay.OutputFileName,
				replay.OutputStdoutFileName,
				replay.OutputStderrFileName,
				replay.DecisionsFileName,
				replay.DispatchFileName,
			} {
				Expect(filepath.Join(dir, name)).To(BeARegularFile(), "%s must assert %s", dir, name)
			}
			dispatchBody, readErr := os.ReadFile(filepath.Join(dir, replay.DispatchFileName))
			Expect(readErr).NotTo(HaveOccurred())
			dispatch := strings.TrimSpace(string(dispatchBody))
			Expect(dispatch).NotTo(BeEmpty(), dir)
			dispatches[dispatch] = dir
		}

		for _, filterPath := range filterPaths {
			if filepath.Base(filterPath) == ".mappings.yaml" {
				continue
			}
			body, readErr := os.ReadFile(filterPath)
			Expect(readErr).NotTo(HaveOccurred())
			definition, parseErr := filteryaml.ParseDefinition(body)
			Expect(parseErr).NotTo(HaveOccurred(), filterPath)
			for _, filterCase := range definition.Cases {
				key := definition.Filter + "|" + filterCase.ID
				Expect(dispatches).To(HaveKey(key), "missing fixture for %s", key)
			}
		}

		mappingsBody, err := os.ReadFile(filepath.Join(root, "filters", ".mappings.yaml"))
		Expect(err).NotTo(HaveOccurred())
		var mappings corpusMappings
		Expect(yaml.Unmarshal(mappingsBody, &mappings)).To(Succeed())
		Expect(mappings.Version).To(Equal(1))
		for alias := range mappings.Map {
			Expect(invoked).To(HaveKey(alias), "missing direct mapping fixture for %s", alias)
		}
	})
})
