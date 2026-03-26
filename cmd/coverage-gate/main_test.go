package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go-command-compression-proxy/internal/quality/coverage"
)

const internalPrefix = "internal/"

var _ = Describe("Coverage gate", func() {
	Describe("parseConfig", func() {
		It("reads the configured flags", func() {
			cfg, code, err := parseConfig([]string{
				"-coverprofile", "coverage.out",
				"-module", "example/module",
				"-internal-prefix", "pkg/internal/",
				"-threshold", "91.5",
				"-summary-out", "summary.md",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(code).To(BeZero())
			Expect(cfg.coverProfile).To(Equal("coverage.out"))
			Expect(cfg.modulePath).To(Equal("example/module"))
			Expect(cfg.internalPrefix).To(Equal("pkg/internal/"))
			Expect(cfg.threshold).To(Equal(91.5))
			Expect(cfg.outPath).To(Equal("summary.md"))
		})

		It("fails when the coverprofile flag is missing", func() {
			_, code, err := parseConfig(nil)

			Expect(err).To(MatchError("coverprofile is required"))
			Expect(code).To(Equal(1))
		})

		It("reports flag parsing errors with exit code 2", func() {
			_, code, err := parseConfig([]string{"--unknown"})

			Expect(err).To(HaveOccurred())
			Expect(code).To(Equal(2))
		})
	})

	Describe("loadCoverageReport", func() {
		It("parses the requested coverage profile", func() {
			path := filepath.Join(GinkgoT().TempDir(), "coverage.out")
			raw := strings.Join([]string{
				"mode: atomic",
				"go-command-compression-proxy/internal/runner/run.go:1.1,2.2 3 1",
				"go-command-compression-proxy/internal/engine/engine.go:1.1,2.2 5 1",
				"go-command-compression-proxy/cmd/ccp/main.go:1.1,2.2 4 1",
			}, "\n")
			Expect(os.WriteFile(path, []byte(raw), 0o644)).To(Succeed())

			report, err := loadCoverageReport(path, "go-command-compression-proxy", internalPrefix, 80)

			Expect(err).NotTo(HaveOccurred())
			Expect(report.InternalPrefix).To(Equal(internalPrefix))
			Expect(report.InternalPackages).To(HaveLen(2))
			Expect(report.OtherPackages).To(HaveLen(1))
			Expect(report.InternalTotal.Statements).To(Equal(int64(8)))
			Expect(report.InternalTotal.Covered).To(Equal(int64(8)))
		})
	})

	Describe("prepareSummaryWriter", func() {
		It("writes to stdout only when no summary path is configured", func() {
			stdout := &strings.Builder{}
			writer, summaryFile, err := prepareSummaryWriter(stdout, "")

			Expect(err).NotTo(HaveOccurred())
			Expect(summaryFile).To(BeNil())
			Expect(writer).To(Equal(io.Writer(stdout)))
		})

		It("writes to both stdout and the requested file", func() {
			path := filepath.Join(GinkgoT().TempDir(), "summary.md")
			stdout := &strings.Builder{}

			writer, summaryFile, err := prepareSummaryWriter(stdout, path)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				Expect(summaryFile.Close()).To(Succeed())
			})

			_, err = io.WriteString(writer, "summary body")
			Expect(err).NotTo(HaveOccurred())

			body, readErr := os.ReadFile(path)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal("summary body"))
			Expect(stdout.String()).To(Equal("summary body"))
		})
	})

	It("renders summary statuses", func() {
		report := coverage.Report{
			InternalPrefix: internalPrefix,
			Threshold:      80,
			InternalTotal: coverage.PackageStat{
				Package:    internalPrefix,
				Covered:    8,
				Statements: 10,
				Percent:    80,
			},
			InternalPackages: []coverage.PackageStat{
				{Package: "internal/runner", Covered: 4, Statements: 5, Percent: 80},
				{Package: "internal/engine", Covered: 3, Statements: 5, Percent: 60},
			},
			OtherPackages: []coverage.PackageStat{
				{Package: "cmd/ccp", Covered: 2, Statements: 4, Percent: 50},
			},
		}

		out := renderSummary(report)
		Expect(out).To(ContainSubstring("Module-group coverage (`internal/`): **80.00%**"))
		Expect(out).To(ContainSubstring("internal/runner"))
		Expect(out).To(ContainSubstring("PASS"))
		Expect(out).To(ContainSubstring("internal/engine"))
		Expect(out).To(ContainSubstring("FAIL"))
		Expect(out).To(ContainSubstring("| `internal/engine` | 60.00% | 3/5 | FAIL |"))
		Expect(out).To(ContainSubstring("Informational packages outside required scope"))
	})

	DescribeTable("validating coverage gates",
		func(report coverage.Report, expected string) {
			err := validateGate(report, 80.0, internalPrefix)
			if strings.TrimSpace(expected) == "" {
				Expect(err).NotTo(HaveOccurred())
				return
			}
			Expect(err).To(MatchError(ContainSubstring(expected)))
		},
		Entry("fails for low module total", coverage.Report{
			InternalTotal: coverage.PackageStat{Percent: 79.99},
		}, "total 79.99% < 80.00%"),
		Entry("fails for package threshold", coverage.Report{
			InternalTotal: coverage.PackageStat{Percent: 85},
			InternalPackages: []coverage.PackageStat{
				{Package: "internal/a", Percent: 90},
				{Package: "internal/b", Percent: 79.5},
				{Package: "internal/c", Percent: 40},
			},
		}, "2 package(s) in internal/ below 80.00%"),
		Entry("passes at threshold", coverage.Report{
			InternalTotal: coverage.PackageStat{Percent: 80},
			InternalPackages: []coverage.PackageStat{
				{Package: "internal/a", Percent: 80},
				{Package: "internal/b", Percent: 99.9},
			},
		}, ""),
	)

	Describe("orderedWriter", func() {
		It("keeps the first write error and stops later writes", func() {
			writer := &stubErrorWriter{err: errors.New("boom")}
			ow := &orderedWriter{w: writer}

			ow.writef("hello %s", "world")
			ow.writes("ignored")

			Expect(ow.err).To(MatchError("boom"))
			Expect(writer.calls).To(Equal(1))
		})
	})

	Describe("run", func() {
		It("writes the summary and exits successfully for a passing report", func() {
			tempDir := GinkgoT().TempDir()
			coverProfile := filepath.Join(tempDir, "coverage.out")
			summaryPath := filepath.Join(tempDir, "summary.md")
			raw := strings.Join([]string{
				"mode: atomic",
				"go-command-compression-proxy/internal/runner/run.go:1.1,2.2 3 1",
				"go-command-compression-proxy/internal/engine/engine.go:1.1,2.2 5 1",
			}, "\n")
			Expect(os.WriteFile(coverProfile, []byte(raw), 0o644)).To(Succeed())

			var stdout strings.Builder
			var stderr strings.Builder

			code := run([]string{
				"-coverprofile", coverProfile,
				"-summary-out", summaryPath,
			}, &stdout, &stderr)

			Expect(code).To(BeZero())
			Expect(stderr.String()).To(BeEmpty())
			Expect(stdout.String()).To(ContainSubstring("## Coverage Gate"))

			summaryBody, err := os.ReadFile(summaryPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(summaryBody)).To(ContainSubstring("Module-group coverage"))
		})

		It("returns a failure code when gate validation fails", func() {
			tempDir := GinkgoT().TempDir()
			coverProfile := filepath.Join(tempDir, "coverage.out")
			raw := strings.Join([]string{
				"mode: atomic",
				"go-command-compression-proxy/internal/runner/run.go:1.1,2.2 3 0",
			}, "\n")
			Expect(os.WriteFile(coverProfile, []byte(raw), 0o644)).To(Succeed())

			var stdout strings.Builder
			var stderr strings.Builder

			code := run([]string{"-coverprofile", coverProfile}, &stdout, &stderr)

			Expect(code).To(Equal(1))
			Expect(stdout.String()).To(ContainSubstring("## Coverage Gate"))
			Expect(stderr.String()).To(ContainSubstring("coverage gate failed"))
		})
	})

	Describe("writeFailure", func() {
		It("prints the formatted message and returns the requested code", func() {
			var stderr strings.Builder

			code := writeFailure(&stderr, 7, "boom %d", 7)

			Expect(code).To(Equal(7))
			Expect(stderr.String()).To(ContainSubstring("boom 7"))
		})
	})
})

type stubErrorWriter struct {
	calls int
	err   error
}

func (w *stubErrorWriter) Write(_ []byte) (int, error) {
	w.calls++
	return 0, w.err
}
