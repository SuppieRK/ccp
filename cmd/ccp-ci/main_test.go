package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go-command-compression-proxy/internal/benchmark"
)

var _ = Describe("ccp-ci", func() {
	Describe("parseConfig", func() {
		It("reads the configured flags", func() {
			cfg, code, err := parseConfig([]string{
				"-fixtures-root", "fixtures",
				"-artifacts-dir", "artifacts",
				"-tool", "grep",
				"-previous-report", "report.json",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(code).To(BeZero())
			Expect(cfg.fixturesRoot).To(Equal("fixtures"))
			Expect(cfg.artifactsDir).To(Equal("artifacts"))
			Expect(cfg.tool).To(Equal("grep"))
			Expect(cfg.previousReport).To(Equal("report.json"))
		})

		It("returns exit code 2 for flag parsing errors", func() {
			_, code, err := parseConfig([]string{"--unknown"})

			Expect(err).To(HaveOccurred())
			Expect(code).To(Equal(2))
		})
	})

	Describe("resolveFixturesRoot", func() {
		DescribeTable("resolving the tool fixture root",
			func(tool string, expectedSuffix string, wantErr string) {
				root := GinkgoT().TempDir()

				toolDir, err := resolveFixturesRoot(root, tool)

				if wantErr != "" {
					Expect(err).To(MatchError(ContainSubstring(wantErr)))
					return
				}
				Expect(err).NotTo(HaveOccurred())
				Expect(os.MkdirAll(toolDir, 0o755)).To(Succeed())
				Expect(toolDir).To(Equal(filepath.Join(root, expectedSuffix)))
			},
			Entry("uses the root when no tool is provided", "", ".", ""),
			Entry("trims surrounding whitespace", " grep ", "grep", ""),
			Entry("scopes fixtures to a selected tool", "grep", "grep", ""),
			Entry("rejects tool names that escape the root", "../outside", "", "single tool directory name"),
			Entry("rejects nested tool names", "tools/grep", "", "single tool directory name"),
		)
	})

	DescribeTable("validating single tool directory names",
		func(tool string, expected bool) {
			Expect(isSingleToolDirName(tool)).To(Equal(expected))
		},
		Entry("plain directory name", "grep", true),
		Entry("current directory", ".", false),
		Entry("parent directory", "..", false),
		Entry("nested unix path", "tools/grep", false),
		Entry("nested windows path", `tools\grep`, false),
	)

	Describe("run", func() {
		BeforeEach(func() {
			prevRun := runBenchmarks
			prevWrite := writeBenchmarkSummary
			prevFailure := benchmarkFailureReport
			DeferCleanup(func() {
				runBenchmarks = prevRun
				writeBenchmarkSummary = prevWrite
				benchmarkFailureReport = prevFailure
			})
		})

		It("runs the benchmark workflow with the resolved fixture root", func() {
			var received benchmark.RunOptions
			runBenchmarks = func(opts benchmark.RunOptions) (benchmark.RunReport, error) {
				received = opts
				return benchmark.RunReport{}, nil
			}
			writeBenchmarkSummary = func(report benchmark.RunReport) error {
				Expect(report.Failed).To(BeFalse())
				return nil
			}
			benchmarkFailureReport = func(report benchmark.RunReport) []string {
				return nil
			}

			var stderr bytes.Buffer
			code := run([]string{
				"-fixtures-root", "fixtures",
				"-artifacts-dir", "artifacts",
				"-tool", "grep",
				"-previous-report", "report.json",
			}, &stderr)

			Expect(code).To(BeZero())
			Expect(stderr.String()).To(BeEmpty())
			Expect(received.FixturesRoot).To(Equal(filepath.Join("fixtures", "grep")))
			Expect(received.ArtifactsDir).To(Equal("artifacts"))
			Expect(received.ProxyBinary).To(Equal("ccp"))
			Expect(received.Timeout).To(Equal(2 * time.Minute))
			Expect(received.PreviousReport).To(Equal("report.json"))
		})

		It("returns a failure code and writes the summary lines when the report fails", func() {
			runBenchmarks = func(opts benchmark.RunOptions) (benchmark.RunReport, error) {
				return benchmark.RunReport{Failed: true}, nil
			}
			writeBenchmarkSummary = func(report benchmark.RunReport) error {
				return nil
			}
			benchmarkFailureReport = func(report benchmark.RunReport) []string {
				return []string{"case failed"}
			}

			var stderr bytes.Buffer
			code := run(nil, &stderr)

			Expect(code).To(Equal(1))
			Expect(stderr.String()).To(ContainSubstring("case failed"))
		})
	})

	Describe("fatal", func() {
		It("prints the message and exits non-zero", func() {
			cmd := exec.Command(os.Args[0], "-test.run=TestCCPCI", "--", "fatal-helper")
			cmd.Env = append(os.Environ(), "CCP_CI_FATAL_HELPER=1")
			var stderr bytes.Buffer
			cmd.Stderr = &stderr

			err := cmd.Run()
			var exitErr *exec.ExitError
			Expect(err).To(HaveOccurred())
			Expect(errors.As(err, &exitErr)).To(BeTrue())
			Expect(exitErr.ExitCode()).To(Equal(1))
			Expect(stderr.String()).To(ContainSubstring("boom"))
		})
	})

	Describe("writeFailure", func() {
		It("prints the message and returns the requested code", func() {
			var stderr bytes.Buffer

			code := writeFailure(&stderr, 7, "boom %s", "again")

			Expect(code).To(Equal(7))
			Expect(stderr.String()).To(ContainSubstring("boom again"))
		})
	})
})

func init() {
	runFatalHelperIfRequested()
}

func runFatalHelperIfRequested() {
	if os.Getenv("CCP_CI_FATAL_HELPER") != "1" {
		return
	}

	fatal("boom")
}
