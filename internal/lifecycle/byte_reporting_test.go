package lifecycle

import (
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/SuppieRK/cmdshape/internal/metrics"
)

var _ = Describe("compatible byte reporting", func() {
	It("formats compact byte totals with accurate human labels", func() {
		total := metrics.SummaryTotal{
			Commands:     88,
			RawBytes:     20_300_000,
			KeptBytes:    352_100,
			DroppedBytes: 19_947_900,
			DropRatio:    0.982655,
		}

		headline := gainHeadline(total, filtersEnvelope{})

		Expect(headline).To(Equal("88 cmds · 19.4 MiB source → 343.8 KiB emitted (98.3% net reduction)"))
		Expect(strings.ToLower(headline)).NotTo(ContainSubstring("token"))
		Expect(strings.ToLower(headline)).NotTo(ContainSubstring("saved"))
	})

	DescribeTable("formats IEC byte sizes",
		func(value int64, expected string) {
			Expect(formatByteSize(value)).To(Equal(expected))
		},
		Entry("bytes", int64(31), "31 B"),
		Entry("kibibytes", int64(1024), "1 KiB"),
		Entry("mebibytes", int64(1024*1024), "1 MiB"),
	)

	It("preserves legacy token estimates in JSON", func() {
		body, err := json.Marshal(summaryEnvelope{
			Dataset: "summary",
			Rows: []metrics.SummaryRow{{
				Command:               "git status",
				Commands:              1,
				RawBytes:              100,
				KeptBytes:             25,
				DroppedBytes:          75,
				DropRatio:             0.75,
				EstimatedInputTokens:  25,
				EstimatedOutputTokens: 7,
				EstimatedSavedTokens:  18,
				EstimatedSavingsPct:   72,
			}},
			Total: metrics.SummaryTotal{
				Commands:     1,
				RawBytes:     100,
				KeptBytes:    25,
				DroppedBytes: 75,
				DropRatio:    0.75,
			},
		})

		Expect(err).NotTo(HaveOccurred())
		text := string(body)
		Expect(text).To(ContainSubstring(`"raw_bytes":100`))
		Expect(text).To(ContainSubstring(`"kept_bytes":25`))
		Expect(text).To(ContainSubstring(`"dropped_bytes":75`))
		Expect(text).To(ContainSubstring(`"drop_ratio":0.75`))
		Expect(text).To(ContainSubstring(`"estimated_input_tokens":25`))
		Expect(text).To(ContainSubstring(`"estimated_output_tokens":7`))
		Expect(text).To(ContainSubstring(`"estimated_saved_tokens":18`))
		Expect(text).To(ContainSubstring(`"estimated_savings_pct":72`))
		Expect(text).NotTo(ContainSubstring("source_bytes"))
		Expect(text).NotTo(ContainSubstring("emitted_bytes"))
	})

	It("preserves the 0.9.2 summary CSV columns", func() {
		output, err := captureStdout(func() error {
			return writeSummaryCSV(
				[]metrics.SummaryRow{{
					Command:               "git status",
					Commands:              1,
					RawBytes:              100,
					KeptBytes:             25,
					DroppedBytes:          75,
					DropRatio:             0.75,
					EstimatedInputTokens:  25,
					EstimatedOutputTokens: 7,
					EstimatedSavedTokens:  18,
					EstimatedSavingsPct:   72,
				}},
				metrics.SummaryTotal{
					Commands:     1,
					RawBytes:     100,
					KeptBytes:    25,
					DroppedBytes: 75,
					DropRatio:    0.75,
				},
				filtersEnvelope{},
			)
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(ContainSubstring("raw_bytes,kept_bytes,dropped_bytes,drop_ratio"))
		Expect(output).To(ContainSubstring("estimated_input_tokens,estimated_output_tokens,estimated_saved_tokens,estimated_savings_pct"))
	})

	It("describes exact human bytes and compatibility estimates separately", func() {
		output, err := captureStderrOutput(func() error {
			return RunGain([]string{"--help"}, "")
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(ContainSubstring("source, emitted, and net-reduction command-output bytes"))
		Expect(output).To(ContainSubstring("0.9.2 estimated-token compatibility fields"))
		Expect(output).To(ContainSubstring("4B/token heuristic"))
		Expect(output).To(ContainSubstring("not model observations"))
	})

	It("keeps every filter-performance CSV row aligned with the 0.9.2 schema", func() {
		const columns = 38

		Expect(filterPerformanceRowCSV(metrics.PerformanceRow{}, filtersEnvelope{})).To(HaveLen(columns))
		Expect(filterPerformanceSuggestionCSV(filterPerformanceSuggestion{}, filtersEnvelope{})).To(HaveLen(columns))
		Expect(filterPerformanceBuildSummaryCSV(metrics.RegistryBuildSummary{}, filtersEnvelope{})).To(HaveLen(columns))
		Expect(filterPerformanceBuildSourceCSV(metrics.RegistrySourceBuildRow{}, filtersEnvelope{})).To(HaveLen(columns))
	})

	DescribeTable("renders signed human byte changes",
		func(sourceBytes, emittedBytes int64, expected string) {
			Expect(byteChangeText(sourceBytes, emittedBytes)).To(Equal(expected))
		},
		Entry("reduction", int64(100), int64(25), "75.0% net reduction"),
		Entry("no change", int64(100), int64(100), "no net byte change"),
		Entry("expansion", int64(100), int64(125), "25.0% expansion"),
		Entry("empty output", int64(0), int64(0), "no net byte change"),
		Entry("generated output", int64(0), int64(5), "new output"),
	)

	It("groups strong, low, and expanding tools without overlap", func() {
		lines := summaryInsightLines([]metrics.SummaryToolRow{
			{Tool: "find", Commands: 4, RawBytes: 1_000, KeptBytes: 200},
			{Tool: "git", Commands: 8, RawBytes: 1_000, KeptBytes: 900},
			{Tool: "summary", Commands: 2, RawBytes: 100, KeptBytes: 140},
		})

		Expect(lines).To(HaveLen(3))
		Expect(lines[0].label).To(Equal("Most net reduction"))
		Expect(lines[0].value).To(ContainSubstring("find"))
		Expect(lines[1].label).To(Equal("Low reduction"))
		Expect(lines[1].value).To(ContainSubstring("git"))
		Expect(lines[2].label).To(Equal("Expansion"))
		Expect(lines[2].value).To(ContainSubstring("summary"))
	})

	DescribeTable("keeps tail formatting coverage after retiring the token helper",
		func(input string, max int, expected string) {
			Expect(truncateTailForDisplay(input, max)).To(Equal(expected))
		},
		Entry("empty limit", "abcdef", 0, ""),
		Entry("keeps short values", "abcdef", 6, "abcdef"),
		Entry("uses raw tails when the limit is tiny", "abcdef", 3, "def"),
		Entry("prefixes longer tails with an ellipsis", "abcdef", 5, "...ef"),
	)
})
