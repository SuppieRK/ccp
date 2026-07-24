package benchmark

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/SuppieRK/cmdshape/internal/replay"
)

var _ = Describe("benchmark fixture corpus", func() {
	It("loads every committed benchmark fixture and validates replay streams when present", func() {
		fixturesRoot := filepath.Join("..", "..", "testdata", "benchmarks")

		cases, err := discoverFixtures(fixturesRoot)
		Expect(err).NotTo(HaveOccurred())
		Expect(cases).NotTo(BeEmpty())

		commandFixtures := map[string]struct{}{}
		walkErr := filepath.WalkDir(fixturesRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || d.Name() != replay.CommandFileName {
				return nil
			}
			commandFixtures[filepath.Dir(path)] = struct{}{}
			return nil
		})
		Expect(walkErr).NotTo(HaveOccurred())
		Expect(cases).To(HaveLen(len(commandFixtures)))

		for _, item := range cases {
			By("validating fixture " + item.tool + "/" + item.name)

			fixture, loadErr := replay.LoadFixture(item.dir)
			Expect(loadErr).NotTo(HaveOccurred(), item.dir)
			Expect(fixture.Command.Argv).NotTo(BeEmpty(), item.dir)

			events, eventsErr := replay.ReadEvents(fixture.StdoutPath, fixture.StderrPath)
			Expect(eventsErr).NotTo(HaveOccurred(), item.dir)

			if replay.HasRequiredFixtureFiles(item.dir) && (fileExists(fixture.StdoutPath) || fileExists(fixture.StderrPath)) {
				Expect(events).NotTo(BeNil(), item.dir)
			}

			relDir, relErr := filepath.Rel(fixturesRoot, item.dir)
			Expect(relErr).NotTo(HaveOccurred())
			Expect(strings.Split(relDir, string(filepath.Separator))).To(HaveLen(2), item.dir)
		}
	})
})

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
