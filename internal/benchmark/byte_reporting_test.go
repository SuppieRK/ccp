package benchmark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/SuppieRK/cmdshape/internal/metrics"
	"github.com/SuppieRK/cmdshape/internal/replay"
)

var _ = Describe("benchmark byte reporting", func() {
	It("persists exact fixture byte counts in gain metrics", func() {
		artifactDir := GinkgoT().TempDir()

		Expect(appendCaseMetrics(artifactDir, []string{"git", "status"}, 7, 3)).To(Succeed())

		total, err := metrics.QuerySummary(metrics.ProjectPath(artifactDir), metrics.QueryOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(total.RawBytes).To(Equal(int64(7)))
		Expect(total.KeptBytes).To(Equal(int64(3)))
		Expect(total.DroppedBytes).To(Equal(int64(4)))
		Expect(total.DropRatio).To(BeNumerically("~", 4.0/7.0, 0.0001))
	})

	It("preserves the 0.9.2 benchmark JSON schema", func() {
		body, err := json.Marshal(CaseResult{
			Tool:                 "git",
			Case:                 "status",
			Command:              "git status",
			NativeTokens:         25,
			ProxyTokens:          7,
			NativeBytes:          100,
			ProxyBytes:           25,
			TokenCompactionRatio: 25.0 / 7.0,
			Success:              true,
		})

		Expect(err).NotTo(HaveOccurred())
		var fields map[string]any
		Expect(json.Unmarshal(body, &fields)).To(Succeed())
		Expect(fields).To(HaveKey("native_tokens"))
		Expect(fields).To(HaveKey("proxy_tokens"))
		Expect(fields).To(HaveKeyWithValue("native_bytes", float64(100)))
		Expect(fields).To(HaveKeyWithValue("proxy_bytes", float64(25)))
		Expect(fields).To(HaveKey("token_compaction_ratio"))
		Expect(fields).NotTo(HaveKey("shaped_bytes"))
		Expect(fields).NotTo(HaveKey("byte_reduction_ratio"))
	})

	It("writes exact byte columns to the benchmark summary", func() {
		summaryPath := filepath.Join(GinkgoT().TempDir(), "summary.md")
		previous := os.Getenv("GITHUB_STEP_SUMMARY")
		Expect(os.Setenv("GITHUB_STEP_SUMMARY", summaryPath)).To(Succeed())
		DeferCleanup(func() {
			if previous == "" {
				_ = os.Unsetenv("GITHUB_STEP_SUMMARY")
				return
			}
			_ = os.Setenv("GITHUB_STEP_SUMMARY", previous)
		})

		Expect(WriteSummary(RunReport{Results: []CaseResult{{
			Tool:        "git",
			Case:        "status",
			Command:     "git status",
			NativeBytes: 100,
			ProxyBytes:  25,
			Success:     true,
		}}})).To(Succeed())

		body, err := os.ReadFile(summaryPath)
		Expect(err).NotTo(HaveOccurred())
		text := string(body)
		Expect(text).To(ContainSubstring("Native bytes | Shaped bytes | Net byte reduction %"))
		Expect(text).To(ContainSubstring("| 100 | 25 | 75.00 |"))
		Expect(text).NotTo(ContainSubstring("token"))
	})

	It("fails shaped-output expansion using exact bytes", func() {
		fixturesRoot := GinkgoT().TempDir()
		artifactsDir := GinkgoT().TempDir()
		fixtureDir := filepath.Join(fixturesRoot, "grep", "expansion")
		writeFixtureFile(fixtureDir, "command.yaml", "argv: [\"grep\"]\nexit_code: 0\n")
		writeFixtureFile(fixtureDir, "stdout.txt", "00000|x\n")

		previousVerifier := runVerifyFixture
		runVerifyFixture = func(_ string, dir string, _ time.Duration) error {
			Expect(os.WriteFile(filepath.Join(dir, replay.VerifyOutputFileName), []byte("expanded\n"), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(dir, replay.VerifyStdoutFileName), []byte("expanded\n"), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(dir, replay.VerifyStderrFileName), nil, 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(dir, replay.VerifyDecisionsFileName), nil, 0o644)).To(Succeed())
			return nil
		}
		DeferCleanup(func() { runVerifyFixture = previousVerifier })

		report, err := Run(RunOptions{
			FixturesRoot: fixturesRoot,
			ArtifactsDir: artifactsDir,
			Timeout:      time.Second,
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(report.Failed).To(BeTrue())
		Expect(report.Results[0].NativeBytes).To(Equal(2))
		Expect(report.Results[0].ProxyBytes).To(Equal(9))
		Expect(report.Results[0].Warnings).To(ContainElement("output expansion: native=2 bytes proxy=9 bytes"))
	})

	It("compares exact byte reduction while retaining legacy token fields", func() {
		current := CaseResult{
			InputHash:   "same",
			NativeBytes: 100,
			ProxyBytes:  40,
		}
		previous := CaseResult{
			InputHash:   "same",
			NativeBytes: 100,
			ProxyBytes:  20,
		}

		maybeWarnCompactionDrop(&current, previous)

		Expect(current.Warnings).To(ConsistOf("net byte reduction dropped from 80.00% to 60.00%"))
		Expect(summaryStatusCell(CaseResult{Success: true, Warnings: current.Warnings})).To(Equal("🟡"))
	})
})
