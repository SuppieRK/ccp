package lifecycle

import (
	"path/filepath"
	"slices"

	"github.com/SuppieRK/cmdshape/internal/metrics"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("filter performance decision coverage", func() {
	It("parses filters and rejects invalid query options", func() {
		flags, handled, err := parseFilterPerformanceFlags([]string{
			"--format", "CSV",
			"--since", "2h",
			"--tool", " git ",
			"--failed",
			"--global",
			"--limit", "0",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(handled).To(BeFalse())
		Expect(flags).To(Equal(filterPerformanceFlags{
			format: "csv",
			since:  "2h",
			tool:   "git",
			failed: true,
			global: true,
			limit:  0,
		}))

		opts, err := buildFilterPerformanceQueryOptions(flags)
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.Tool).To(Equal("git"))
		Expect(opts.Failed).To(BeTrue())
		Expect(opts.Since).NotTo(BeZero())

		_, err = buildFilterPerformanceQueryOptions(filterPerformanceFlags{since: "nonsense"})
		Expect(err).To(HaveOccurred())
		_, _, err = parseFilterPerformanceFlags([]string{"--limit", "-2"})
		Expect(err).To(MatchError("invalid --limit -2 (expected -1, 0, or a positive integer)"))
	})

	It("orders rows through every deterministic tie breaker", func() {
		rows := []metrics.PerformanceRow{
			{Tool: "z", Filter: "same", Case: "b", Commands: 1, EstimatedSavedTokens: 2},
			{Tool: "a", Filter: "z", Case: "a", Commands: 1, EstimatedSavedTokens: 2},
			{Tool: "a", Filter: "a", Case: "b", Commands: 1, EstimatedSavedTokens: 2},
			{Tool: "a", Filter: "a", Case: "a", Commands: 1, EstimatedSavedTokens: 2},
			{Tool: "x", Commands: 1, EstimatedSavedTokens: 3},
			{Tool: "y", Commands: 2},
		}

		slices.SortFunc(rows, comparePerformanceRows)

		Expect(rows[0].Tool).To(Equal("y"))
		Expect(rows[1].Tool).To(Equal("x"))
		Expect(rows[2].Case).To(Equal("a"))
		Expect(rows[3].Case).To(Equal("b"))
		Expect(rows[4].Filter).To(Equal("z"))
		Expect(rows[5].Tool).To(Equal("z"))
	})

	It("caps each actionable suggestion kind and ignores non-actionable rows", func() {
		rows := make([]metrics.PerformanceRow, 0, maxPerformanceHints+2)
		missed := make([]metrics.MissedOpportunity, 0, maxPerformanceHints+2)
		buildRows := make([]metrics.RegistrySourceBuildRow, 0, maxPerformanceHints+3)
		for index := range maxPerformanceHints + 2 {
			rows = append(rows, metrics.PerformanceRow{
				Tool:                "tool",
				Filter:              "filter",
				Case:                string(rune('a' + index)),
				Commands:            2,
				FailedCommands:      1,
				FailedRate:          0.5,
				EstimatedSavingsPct: 1,
			})
			missed = append(missed, metrics.MissedOpportunity{Command: "native", Count: int64(index + 1)})
			buildRows = append(buildRows, metrics.RegistrySourceBuildRow{
				SourceDir:     "/source",
				Builds:        1,
				AvgDurationMS: float64(index + 1),
			})
		}
		buildRows = append(buildRows,
			metrics.RegistrySourceBuildRow{SourceDir: "/zero-builds", AvgDurationMS: 1},
			metrics.RegistrySourceBuildRow{SourceDir: "/zero-duration", Builds: 1},
		)

		suggestions := buildFilterPerformanceSuggestions(rows, missed, buildRows)

		Expect(countKind(suggestions, suggestionReviewCase)).To(Equal(maxPerformanceHints))
		Expect(countKind(suggestions, suggestionFailureHeavy)).To(Equal(maxPerformanceHints))
		Expect(countKind(suggestions, suggestionPassthrough)).To(Equal(maxPerformanceHints))
		Expect(countKind(suggestions, suggestionRegistryCost)).To(Equal(maxPerformanceHints))
	})

	It("aggregates and limits passthrough opportunities across healthy sources", func() {
		root := GinkgoT().TempDir()
		first := filepath.Join(root, "first.db")
		second := filepath.Join(root, "second.db")
		for _, metric := range []metrics.RunMetric{
			{Command: "git status", Passthrough: true},
			{Command: "git status", Passthrough: true},
			{Command: "go test", Passthrough: true},
			{Command: "ignored", Passthrough: false},
		} {
			Expect(metrics.Append(first, metric)).To(Succeed())
		}
		Expect(metrics.Append(second, metrics.RunMetric{Command: "git status", Passthrough: true})).To(Succeed())
		session := &globalQuerySession{
			sources: []globalMetricsSource{
				{CWD: "/first", MetricsPath: first},
				{CWD: "/second", MetricsPath: second},
			},
			failures: map[string]globalQueryFailure{},
		}

		rows, err := queryGlobalMissedOpportunities(session, metrics.QueryOptions{}, 1)

		Expect(err).NotTo(HaveOccurred())
		Expect(rows).To(Equal([]metrics.MissedOpportunity{{Command: "git status", Count: 3}}))
		Expect(session.failures).To(BeEmpty())
	})
})
