package main

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go-command-compression-proxy/internal/quality/coverage"
)

const internalPrefix = "internal/"

var _ = Describe("Coverage gate", func() {
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
})
