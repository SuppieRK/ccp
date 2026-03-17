package benchmark

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	"go-command-compression-proxy/internal/replay"
)

var _ = Describe("benchmark replay runner", func() {
	var (
		root      string
		artifacts string
	)

	BeforeEach(func() {
		root = GinkgoT().TempDir()
		artifacts = GinkgoT().TempDir()
	})

	Describe("fixture discovery", func() {
		It("discovers command.yaml fixture directories under tool/case paths", func() {
			fixtureDir := filepath.Join(root, "grep", "recursive-match")
			writeFixtureFile(fixtureDir, "command.yaml", "argv: [\"grep\", \"-r\", \"needle\", \".\"]\n")
			writeFixtureFile(fixtureDir, "output.txt", "out\n")

			cases, err := discoverFixtures(root)

			Expect(err).NotTo(HaveOccurred())
			Expect(cases).To(Equal([]fixtureCase{{
				tool: "grep",
				name: "recursive-match",
				dir:  fixtureDir,
			}}))
		})

		It("supports tool-scoped roots where case directories are direct children", func() {
			fixtureDir := filepath.Join(root, "recursive-match")
			writeFixtureFile(fixtureDir, "command.yaml", "argv: [\"grep\", \"-r\", \"needle\", \".\"]\n")

			cases, err := discoverFixtures(root)

			Expect(err).NotTo(HaveOccurred())
			Expect(cases).To(Equal([]fixtureCase{{
				tool: filepath.Base(root),
				name: "recursive-match",
				dir:  fixtureDir,
			}}))
		})
	})

	DescribeTable("compares expected files when present",
		func(expected string, actual string, label string, matcher OmegaMatcher) {
			tempDir := GinkgoT().TempDir()
			expectedPath := filepath.Join(tempDir, "expected.txt")
			actualPath := filepath.Join(tempDir, "actual.txt")
			if expected != "" {
				Expect(os.WriteFile(expectedPath, []byte(expected), 0o644)).To(Succeed())
			}
			if actual != "" {
				Expect(os.WriteFile(actualPath, []byte(actual), 0o644)).To(Succeed())
			}

			Expect(compareIfPresent(expectedPath, actualPath, label)).To(matcher)
		},
		Entry("missing expected file is ignored", "", "actual\n", "output", BeEmpty()),
		Entry("mismatch is reported", "expected\n", "actual\n", "output", Equal("output mismatch")),
		Entry("missing verify output is reported", "expected\n", "", "output", ContainSubstring("read verify output:")),
	)

	DescribeTable("applies defaults",
		func(input RunOptions, expected RunOptions) {
			Expect(withDefaults(input)).To(Equal(expected))
		},
		Entry("fills all defaults", RunOptions{}, RunOptions{
			FixturesRoot: filepath.Join("testdata", "benchmarks"),
			ArtifactsDir: filepath.Join(".artifacts", "benchmark"),
			ProxyBinary:  "ccp",
			Timeout:      2 * time.Minute,
		}),
		Entry("preserves provided values", RunOptions{
			FixturesRoot: "fixtures",
			ArtifactsDir: "artifacts",
			ProxyBinary:  "proxy",
			Timeout:      5 * time.Second,
		}, RunOptions{
			FixturesRoot: "fixtures",
			ArtifactsDir: "artifacts",
			ProxyBinary:  "proxy",
			Timeout:      5 * time.Second,
		}),
	)

	Describe("run", func() {
		It("copies fixture inputs, invokes verify, compares expected files, and records metrics", func() {
			fixtureDir := filepath.Join(root, "grep", "recursive-match")
			writeFixtureFile(fixtureDir, "command.yaml", "argv: [\"grep\", \"-r\", \"needle\", \"./internal\"]\n")
			writeFixtureFile(fixtureDir, "stdout.txt", "00000|match one\n00001|match two\n")
			writeFixtureFile(fixtureDir, "output.txt", "grouped output\n")
			writeFixtureFile(fixtureDir, "decisions.txt", "<keep>    | match one\n")

			prev := runVerifyFixture
			runVerifyFixture = func(_ string, dir string, _ time.Duration) error {
				Expect(os.WriteFile(filepath.Join(dir, "verify-output.txt"), []byte("grouped output\n"), 0o644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(dir, "verify-decisions.txt"), []byte("<keep>    | match one\n"), 0o644)).To(Succeed())
				return nil
			}
			DeferCleanup(func() { runVerifyFixture = prev })

			report, err := Run(RunOptions{
				FixturesRoot: root,
				ArtifactsDir: artifacts,
				ProxyBinary:  "ccp",
				Timeout:      time.Second,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(report.Failed).To(BeFalse())
			Expect(report.Results).To(HaveLen(1))
			Expect(report.Results[0].Tool).To(Equal("grep"))
			Expect(report.Results[0].Case).To(Equal("recursive-match"))
			Expect(report.Results[0].Success).To(BeTrue())
			Expect(report.Results[0].NativeTokens).To(BeNumerically(">", 0))
			Expect(report.Results[0].ProxyTokens).To(BeNumerically(">", 0))
			Expect(filepath.Join(artifacts, "grep", "recursive-match", "command.yaml")).To(BeAnExistingFile())
			Expect(filepath.Join(artifacts, "grep", "recursive-match", "verify-output.txt")).To(BeAnExistingFile())
			Expect(filepath.Join(artifacts, "grep", "recursive-match", ".ccp", "gain.db")).To(BeAnExistingFile())
		})

		DescribeTable("surfaces replay failures as warnings",
			func(setup func(string), verifyErr error, warningMatcher OmegaMatcher) {
				fixtureDir := filepath.Join(root, "grep", "case")
				setup(fixtureDir)

				prev := runVerifyFixture
				runVerifyFixture = func(_ string, dir string, _ time.Duration) error {
					if verifyErr == nil {
						Expect(os.WriteFile(filepath.Join(dir, "verify-output.txt"), []byte("ok\n"), 0o644)).To(Succeed())
						Expect(os.WriteFile(filepath.Join(dir, "verify-decisions.txt"), []byte("<keep>    | ok\n"), 0o644)).To(Succeed())
					}
					return verifyErr
				}
				DeferCleanup(func() { runVerifyFixture = prev })

				report, err := Run(RunOptions{
					FixturesRoot: root,
					ArtifactsDir: artifacts,
					ProxyBinary:  "ccp",
					Timeout:      time.Second,
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(report.Results).To(HaveLen(1))
				Expect(report.Results[0]).To(MatchFields(IgnoreExtras, Fields{
					"Success":  BeFalse(),
					"Warnings": warningMatcher,
				}))
			},
			Entry("invalid fixture is reported during load", func(dir string) {
				writeFixtureFile(dir, "command.yaml", "argv: [\"grep\"]\n")
			}, nil, ConsistOf(ContainSubstring("must contain at least one of stdout.txt, stderr.txt, or output.txt"))),
			Entry("verify subprocess failures are reported", func(dir string) {
				writeFixtureFile(dir, "command.yaml", "argv: [\"grep\"]\n")
				writeFixtureFile(dir, "stdout.txt", "00000|line\n")
				writeFixtureFile(dir, "output.txt", "ok\n")
			}, errors.New("boom"), Equal([]string{"verify failed: boom"})),
			Entry("output mismatches fail the case", func(dir string) {
				writeFixtureFile(dir, "command.yaml", "argv: [\"grep\"]\n")
				writeFixtureFile(dir, "stdout.txt", "00000|line\n")
				writeFixtureFile(dir, "output.txt", "expected\n")
			}, nil, Equal([]string{"output mismatch"})),
		)

		It("reports missing verify output files", func() {
			fixtureDir := filepath.Join(root, "grep", "case")
			writeFixtureFile(fixtureDir, "command.yaml", "argv: [\"grep\"]\n")
			writeFixtureFile(fixtureDir, "stdout.txt", "00000|line\n")
			writeFixtureFile(fixtureDir, "output.txt", "expected\n")

			prev := runVerifyFixture
			runVerifyFixture = func(_ string, _ string, _ time.Duration) error { return nil }
			DeferCleanup(func() { runVerifyFixture = prev })

			report, err := Run(RunOptions{
				FixturesRoot: root,
				ArtifactsDir: artifacts,
				ProxyBinary:  "ccp",
				Timeout:      time.Second,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(report.Results).To(HaveLen(1))
			Expect(report.Results[0].Warnings).To(ContainElement(ContainSubstring("read verify output:")))
		})

		It("marks report failed when any case fails and writes report.json", func() {
			goodDir := filepath.Join(root, "grep", "good")
			badDir := filepath.Join(root, "grep", "bad")
			writeFixtureFile(goodDir, "command.yaml", "argv: [\"grep\"]\n")
			writeFixtureFile(goodDir, "stdout.txt", "00000|line\n")
			writeFixtureFile(goodDir, "output.txt", "ok\n")
			writeFixtureFile(badDir, "command.yaml", "argv: [\"grep\"]\n")

			prev := runVerifyFixture
			runVerifyFixture = func(_ string, dir string, _ time.Duration) error {
				Expect(os.WriteFile(filepath.Join(dir, "verify-output.txt"), []byte("ok\n"), 0o644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(dir, "verify-decisions.txt"), []byte("<keep>    | ok\n"), 0o644)).To(Succeed())
				return nil
			}
			DeferCleanup(func() { runVerifyFixture = prev })

			report, err := Run(RunOptions{
				FixturesRoot: root,
				ArtifactsDir: artifacts,
				ProxyBinary:  "ccp",
				Timeout:      time.Second,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(report.Failed).To(BeTrue())
			Expect(filepath.Join(artifacts, "report.json")).To(BeAnExistingFile())
		})
	})

	Describe("report helpers", func() {
		It("summarizes failures and hashes combined input", func() {
			report := RunReport{
				Results: []CaseResult{
					{Tool: "grep", Case: "ok", Success: true},
					{Tool: "grep", Case: "bad", Success: false, Warnings: []string{"output mismatch"}},
				},
			}

			Expect(FailureSummary(report)).To(Equal([]string{"grep/bad: output mismatch"}))
			Expect(HashInput(nil)).To(HaveLen(64))
			Expect(estimateTokens("")).To(BeZero())
			Expect(estimateTokens("abcd")).To(Equal(1))
			Expect(estimateTokens("abcde")).To(Equal(2))
		})
	})

	Describe("file helpers", func() {
		It("copies fixture inputs when present", func() {
			srcDir := GinkgoT().TempDir()
			dstDir := GinkgoT().TempDir()
			fixture := replayFixture(srcDir)

			Expect(copyFixtureInputs(fixture, dstDir)).To(Succeed())
			Expect(filepath.Join(dstDir, "command.yaml")).To(BeAnExistingFile())
			Expect(filepath.Join(dstDir, "stdout.txt")).To(BeAnExistingFile())
			Expect(filepath.Join(dstDir, "stderr.txt")).To(BeAnExistingFile())
		})

		It("treats missing and directory sources as no-ops", func() {
			tempDir := GinkgoT().TempDir()
			Expect(copyIfPresent(filepath.Join(tempDir, "missing.txt"), filepath.Join(tempDir, "out.txt"))).To(Succeed())
			Expect(os.Mkdir(filepath.Join(tempDir, "adir"), 0o755)).To(Succeed())
			Expect(copyIfPresent(filepath.Join(tempDir, "adir"), filepath.Join(tempDir, "out.txt"))).To(Succeed())
		})

		It("reports writeReportJSON errors for invalid destinations", func() {
			err := writeReportJSON(filepath.Join(GinkgoT().TempDir(), "missing", "nested"), RunReport{})
			Expect(err).To(MatchError(ContainSubstring("open")))
		})
	})
})

func writeFixtureFile(dir string, name string, contents string) {
	Expect(os.MkdirAll(dir, 0o755)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644)).To(Succeed())
}

func replayFixture(dir string) replay.Fixture {
	writeFixtureFile(dir, "command.yaml", "argv: [\"grep\"]\n")
	writeFixtureFile(dir, "stdout.txt", "00000|line\n")
	writeFixtureFile(dir, "stderr.txt", "00001|err\n")
	return replay.Fixture{
		CommandPath: filepath.Join(dir, "command.yaml"),
		StdoutPath:  filepath.Join(dir, "stdout.txt"),
		StderrPath:  filepath.Join(dir, "stderr.txt"),
	}
}
