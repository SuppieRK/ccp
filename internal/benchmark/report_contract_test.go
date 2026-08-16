package benchmark

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("benchmark report contracts", func() {
	It("appends multiple invocations under one byte-oriented table header", func() {
		summaryPath := filepath.Join(GinkgoT().TempDir(), "summary.md")
		previous, hadPrevious := os.LookupEnv("GITHUB_STEP_SUMMARY")
		Expect(os.Setenv("GITHUB_STEP_SUMMARY", summaryPath)).To(Succeed())
		DeferCleanup(func() {
			if hadPrevious {
				_ = os.Setenv("GITHUB_STEP_SUMMARY", previous)
				return
			}
			_ = os.Unsetenv("GITHUB_STEP_SUMMARY")
		})

		Expect(WriteSummary(RunReport{Results: []CaseResult{{
			Tool: "grep", Case: "compact", Command: "grep needle .", NativeBytes: 100, ProxyBytes: 40, Success: true,
		}}})).To(Succeed())
		Expect(WriteSummary(RunReport{Results: []CaseResult{{
			Tool: "grep", Case: "mismatch", Command: "grep missing .", NativeBytes: 20, ProxyBytes: 20,
			Warnings: []string{"output mismatch"},
		}}})).To(Succeed())

		body, err := os.ReadFile(summaryPath)
		Expect(err).NotTo(HaveOccurred())
		text := string(body)
		Expect(strings.Count(text, summaryTableHeaderRow())).To(Equal(1))
		Expect(text).To(ContainSubstring("| 🟢 | grep/compact | `grep needle .` | 100 | 40 | 60.00 |  |"))
		Expect(text).To(ContainSubstring("| 🔴 | grep/mismatch | `grep missing .` | 20 | 20 | 0.00 | output mismatch |"))
	})

	DescribeTable("compares byte-reduction regressions only for identical fixture inputs",
		func(current, previous CaseResult, expectedWarnings []string) {
			maybeWarnCompactionDrop(&current, previous)
			Expect(current.Warnings).To(Equal(expectedWarnings))
		},
		Entry("ignores missing previous reduction", CaseResult{InputHash: "same", NativeBytes: 100, ProxyBytes: 50}, CaseResult{InputHash: "same"}, nil),
		Entry("ignores changed fixture input", CaseResult{InputHash: "new", NativeBytes: 100, ProxyBytes: 60}, CaseResult{InputHash: "old", NativeBytes: 100, ProxyBytes: 40}, nil),
		Entry("does not warn at the exact five-percent boundary", CaseResult{InputHash: "same", NativeBytes: 100, ProxyBytes: 43}, CaseResult{InputHash: "same", NativeBytes: 100, ProxyBytes: 40}, nil),
		Entry("warns past the five-percent boundary", CaseResult{InputHash: "same", NativeBytes: 100, ProxyBytes: 44}, CaseResult{InputHash: "same", NativeBytes: 100, ProxyBytes: 40}, []string{"net byte reduction dropped from 60.00% to 56.00%"}),
	)

	DescribeTable("reports exact byte reduction including expansion",
		func(result CaseResult, expected float64) {
			Expect(byteReductionPct(result)).To(BeNumerically("~", expected, 1e-12))
		},
		Entry("empty native output", CaseResult{NativeBytes: 0, ProxyBytes: 10}, 0.0),
		Entry("exact compaction", CaseResult{NativeBytes: 100, ProxyBytes: 25}, 75.0),
		Entry("output expansion", CaseResult{NativeBytes: 100, ProxyBytes: 110}, -10.0),
	)
})
