package metrics

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	metricsGoTestCommand = "go test ./..."
	gitignoreFileName    = ".gitignore"
	gainDBFileName       = "gain.db"
)

var _ = Describe("metrics storage", func() {
	var tempDir string

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
	})

	It("appends metrics and loads a summary", func() {
		path := filepath.Join(tempDir, "metrics", "runs.db")

		Expect(Append(path, RunMetric{
			Tool:      "go",
			Command:   metricsGoTestCommand,
			RawBytes:  10,
			KeptBytes: 4,
			ExitCode:  0,
		})).To(Succeed())
		Expect(Append(path, RunMetric{
			Tool:      "git",
			Command:   "git status",
			RawBytes:  6,
			KeptBytes: 3,
			ExitCode:  1,
		})).To(Succeed())

		got, err := LoadSummary(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Runs).To(Equal(2))
		Expect(got.RawLines).To(Equal(16))
		Expect(got.KeptLines).To(Equal(7))
		Expect(got.Dropped).To(Equal(9))
		Expect(got.DropRatio).To(Equal(9.0 / 16.0))
	})

	DescribeTable("loading a zero summary",
		func(path string) {
			if path == "__missing__" {
				path = filepath.Join(tempDir, "missing.db")
			}
			got, err := LoadSummary(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(Summary{}))
		},
		Entry("for a missing file", "__missing__"),
		Entry("for an empty path", ""),
	)

	It("noops append with an empty path", func() {
		Expect(Append("", RunMetric{Tool: "noop", RawBytes: 1, KeptBytes: 1})).To(Succeed())
	})

	It("fails when the parent path is a file", func() {
		parentFile := filepath.Join(tempDir, "not-a-dir")
		Expect(os.WriteFile(parentFile, []byte("x"), 0o644)).To(Succeed())

		err := Append(filepath.Join(parentFile, "metrics.db"), RunMetric{Tool: "go", RawBytes: 1, KeptBytes: 1})
		Expect(err).To(HaveOccurred())
	})

	It("fails when the target path is a directory", func() {
		err := Append(tempDir, RunMetric{Tool: "go", RawBytes: 1, KeptBytes: 1})
		Expect(err).To(HaveOccurred())
	})

	It("opens writable metrics databases with durable sync enabled", func() {
		path := filepath.Join(tempDir, "metrics.db")

		db, err := openDB(path, false)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(db.Close()).To(Succeed())
		})

		Expect(db.NoSync).To(BeFalse())
	})

	It("truncates long command text deterministically", func() {
		path := filepath.Join(tempDir, "metrics.db")
		long := strings.Repeat("x", 2000)

		Expect(Append(path, RunMetric{
			Tool:      "go",
			Command:   long,
			RawBytes:  100,
			KeptBytes: 25,
			ExitCode:  0,
		})).To(Succeed())

		history, err := QueryHistory(path, QueryOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(HaveLen(1))
		Expect(len([]rune(history[0].Command))).To(Equal(1024))
		Expect(history[0].Command).To(HaveSuffix("..."))
	})

	It("preserves negative exit codes in failed history queries", func() {
		path := filepath.Join(tempDir, "metrics.db")

		Expect(Append(path, RunMetric{
			Tool:      "node",
			Command:   "node crashed.js",
			RawBytes:  12,
			KeptBytes: 12,
			ExitCode:  -1,
		})).To(Succeed())

		failedHistory, err := QueryHistory(path, QueryOptions{Failed: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(failedHistory).To(HaveLen(1))
		Expect(failedHistory[0].ExitCode).To(Equal(-1))
		Expect(failedHistory[0].Failed).To(BeTrue())
	})

	Context("when storing tool names", func() {
		var path string

		BeforeEach(func() {
			path = filepath.Join(tempDir, "metrics.db")
		})

		It("preserves an explicit tool name", func() {
			Expect(Append(path, RunMetric{
				Tool:        "git",
				Command:     "git ls-files --stage",
				RawBytes:    10,
				KeptBytes:   10,
				Passthrough: true,
			})).To(Succeed())

			history, err := QueryHistory(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(HaveLen(1))
			Expect(history[0].Tool).To(Equal("git"))
			Expect(history[0].Passthrough).To(BeTrue())
		})

		It("normalizes a blank tool name to unknown", func() {
			Expect(Append(path, RunMetric{
				Command:     "echo a && echo b",
				RawBytes:    4,
				KeptBytes:   4,
				Passthrough: true,
			})).To(Succeed())

			history, err := QueryHistory(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(HaveLen(1))
			Expect(history[0].Tool).To(Equal("unknown"))
		})
	})

	DescribeTable("updating local project gitignore for metrics db",
		func(initialGitignore string, expectedGitignore string) {
			project := initGitProjectForMetrics(tempDir, initialGitignore)
			path := filepath.Join(project, ".ccp", gainDBFileName)

			Expect(Append(path, RunMetric{Tool: "go", Command: metricsGoTestCommand, RawBytes: 16, KeptBytes: 8})).To(Succeed())

			if expectedGitignore == "" {
				_, err := os.Stat(filepath.Join(project, gitignoreFileName))
				Expect(err).To(MatchError(os.ErrNotExist))
				return
			}

			b, err := os.ReadFile(filepath.Join(project, gitignoreFileName))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(expectedGitignore))
		},
		Entry("appends .ccp", "node_modules/\n", "node_modules/\n.ccp\n"),
		Entry("does not duplicate .ccp", ".ccp\n", ".ccp\n"),
		Entry("skips when gitignore is missing", "", ""),
	)
})

func initGitProjectForMetrics(project string, gitignoreContent string) string {
	Expect(os.Mkdir(filepath.Join(project, ".git"), 0o755)).To(Succeed())
	if gitignoreContent != "" {
		Expect(os.WriteFile(filepath.Join(project, gitignoreFileName), []byte(gitignoreContent), 0o644)).To(Succeed())
	}
	return project
}
