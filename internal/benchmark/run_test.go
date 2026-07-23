package benchmark

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/metrics"
	"go-command-compression-proxy/internal/replay"
	"go-command-compression-proxy/internal/workspaces"
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

		It("ignores directories without command fixtures", func() {
			Expect(os.MkdirAll(filepath.Join(root, "grep", "empty"), 0o755)).To(Succeed())

			cases, err := discoverFixtures(root)

			Expect(err).NotTo(HaveOccurred())
			Expect(cases).To(BeEmpty())
		})
	})

	DescribeTable("compares required expected files",
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

			Expect(compareRequired(expectedPath, actualPath, label)).To(matcher)
		},
		Entry("missing expected file is reported", "", "actual\n", "output", ContainSubstring("read output fixture:")),
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
		Entry("treats whitespace-only string options as unset", RunOptions{
			FixturesRoot: "  ",
			ArtifactsDir: "\t",
			ProxyBinary:  " ",
		}, RunOptions{
			FixturesRoot: filepath.Join("testdata", "benchmarks"),
			ArtifactsDir: filepath.Join(".artifacts", "benchmark"),
			ProxyBinary:  "ccp",
			Timeout:      2 * time.Minute,
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
			Expect(report.Results[0].InputHash).To(HaveLen(64))
			Expect(report.Results[0].Success).To(BeTrue())
			Expect(report.Results[0].NativeTokens).To(BeNumerically(">", 0))
			Expect(report.Results[0].ProxyTokens).To(BeNumerically(">", 0))
			Expect(filepath.Join(artifacts, "grep", "recursive-match", "command.yaml")).To(BeAnExistingFile())
			Expect(filepath.Join(artifacts, "grep", "recursive-match", "verify-output.txt")).To(BeAnExistingFile())
			Expect(filepath.Join(artifacts, "grep", "recursive-match", ".ccp", "gain.db")).To(BeAnExistingFile())
		})

		It("keeps benchmark metrics local to the artifact gain database", func() {
			fixtureDir := filepath.Join(root, "grep", "recursive-match")
			writeFixtureFile(fixtureDir, "command.yaml", "argv: [\"grep\", \"-r\", \"needle\", \"./internal\"]\n")
			writeFixtureFile(fixtureDir, "stdout.txt", "00000|match one\n")
			writeFixtureFile(fixtureDir, "output.txt", "grouped output\n")

			prev := runVerifyFixture
			runVerifyFixture = func(_ string, dir string, _ time.Duration) error {
				Expect(os.WriteFile(filepath.Join(dir, "verify-output.txt"), []byte("grouped output\n"), 0o644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(dir, "verify-decisions.txt"), []byte("<keep>    | match one\n"), 0o644)).To(Succeed())
				return nil
			}
			DeferCleanup(func() { runVerifyFixture = prev })

			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)

			_, err := Run(RunOptions{
				FixturesRoot: root,
				ArtifactsDir: artifacts,
				ProxyBinary:  "ccp",
				Timeout:      time.Second,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(filepath.Join(artifacts, "grep", "recursive-match", ".ccp", "gain.db")).To(BeAnExistingFile())
			registryPath, pathErr := workspaces.DefaultPath()
			Expect(pathErr).NotTo(HaveOccurred())
			_, statErr := os.Stat(registryPath)
			Expect(statErr).To(MatchError(os.ErrNotExist))
		})

		It("fails the case when artifact metrics cannot be persisted", func() {
			fixtureDir := filepath.Join(root, "grep", "recursive-match")
			writeFixtureFile(fixtureDir, "command.yaml", "argv: [\"grep\", \"-r\", \"needle\", \"./internal\"]\n")
			writeFixtureFile(fixtureDir, "stdout.txt", "00000|match one\n")
			writeFixtureFile(fixtureDir, "output.txt", "grouped output\n")

			prev := runVerifyFixture
			runVerifyFixture = func(_ string, dir string, _ time.Duration) error {
				Expect(os.WriteFile(filepath.Join(dir, "verify-output.txt"), []byte("grouped output\n"), 0o644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(dir, "verify-decisions.txt"), []byte("<keep>    | match one\n"), 0o644)).To(Succeed())
				return nil
			}
			DeferCleanup(func() { runVerifyFixture = prev })

			artifactCaseDir := filepath.Join(artifacts, "grep", "recursive-match")
			Expect(os.MkdirAll(artifactCaseDir, 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(artifactCaseDir, ".ccp"), []byte("block"), 0o644)).To(Succeed())

			report, err := Run(RunOptions{
				FixturesRoot: root,
				ArtifactsDir: artifacts,
				ProxyBinary:  "ccp",
				Timeout:      time.Second,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(report.Results).To(HaveLen(1))
			Expect(report.Results[0].Success).To(BeFalse())
			Expect(report.Results[0].Warnings).To(ContainElement(ContainSubstring("persist benchmark metrics")))
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

		It("fails exact-byte expansion even when no expected output is authored", func() {
			fixtureDir := filepath.Join(root, "grep", "expansion")
			writeFixtureFile(fixtureDir, "command.yaml", "argv: [\"grep\"]\nexit_code: 0\n")
			writeFixtureFile(fixtureDir, "stdout.txt", "00000|x\n")

			prev := runVerifyFixture
			runVerifyFixture = func(_ string, dir string, _ time.Duration) error {
				Expect(os.WriteFile(filepath.Join(dir, replay.VerifyOutputFileName), []byte("expanded\n"), 0o644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(dir, replay.VerifyStdoutFileName), []byte("expanded\n"), 0o644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(dir, replay.VerifyStderrFileName), nil, 0o644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(dir, replay.VerifyDecisionsFileName), nil, 0o644)).To(Succeed())
				return nil
			}
			DeferCleanup(func() { runVerifyFixture = prev })

			report, err := Run(RunOptions{FixturesRoot: root, ArtifactsDir: artifacts, Timeout: time.Second})

			Expect(err).NotTo(HaveOccurred())
			Expect(report.Failed).To(BeTrue())
			Expect(report.Results[0].NativeBytes).To(Equal(2))
			Expect(report.Results[0].ProxyBytes).To(Equal(9))
			Expect(report.Results[0].Warnings).To(ContainElement("output expansion: native=2 bytes proxy=9 bytes"))
			Expect(report.Results[0].Unasserted).To(ConsistOf(
				"output expectation missing",
				"decisions expectation missing",
				"dispatch expectation missing",
			))
		})

		It("detects stream relocation even when merged bytes match", func() {
			fixtureDir := filepath.Join(root, "demo", "stream-relocation")
			writeFixtureFile(fixtureDir, "command.yaml", "argv: [\"demo\"]\nexit_code: 0\n")
			writeFixtureFile(fixtureDir, "stderr.txt", "00000|diagnostic\n")
			writeFixtureFile(fixtureDir, replay.OutputStdoutFileName, "")
			writeFixtureFile(fixtureDir, replay.OutputStderrFileName, "diagnostic\n")
			writeFixtureFile(fixtureDir, replay.DecisionsFileName, "<keep>    | diagnostic\n")
			writeFixtureFile(fixtureDir, replay.DispatchFileName, "demo|default\n")

			prev := runVerifyFixture
			runVerifyFixture = func(_ string, dir string, _ time.Duration) error {
				Expect(os.WriteFile(filepath.Join(dir, replay.VerifyOutputFileName), []byte("diagnostic\n"), 0o644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(dir, replay.VerifyStdoutFileName), []byte("diagnostic\n"), 0o644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(dir, replay.VerifyStderrFileName), nil, 0o644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(dir, replay.VerifyDecisionsFileName), []byte("<keep>    | diagnostic\n"), 0o644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(dir, replay.VerifyDispatchFileName), []byte("demo|default\n"), 0o644)).To(Succeed())
				return nil
			}
			DeferCleanup(func() { runVerifyFixture = prev })

			report, err := Run(RunOptions{FixturesRoot: root, ArtifactsDir: artifacts, Timeout: time.Second})

			Expect(err).NotTo(HaveOccurred())
			Expect(report.Failed).To(BeTrue())
			Expect(report.Results[0].Warnings).To(ConsistOf("stdout mismatch", "stderr mismatch"))
			Expect(report.Results[0].Unasserted).To(BeEmpty())
		})

		It("reports every omitted assertion without treating disclosure as a mismatch", func() {
			fixtureDir := filepath.Join(root, "demo", "unasserted")
			writeFixtureFile(fixtureDir, "command.yaml", "argv: [\"demo\"]\n")
			writeFixtureFile(fixtureDir, "stdout.txt", "00000|native\n")

			prev := runVerifyFixture
			runVerifyFixture = func(_ string, dir string, _ time.Duration) error {
				Expect(os.WriteFile(filepath.Join(dir, replay.VerifyOutputFileName), []byte("native\n"), 0o644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(dir, replay.VerifyStdoutFileName), []byte("native\n"), 0o644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(dir, replay.VerifyStderrFileName), nil, 0o644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(dir, replay.VerifyDecisionsFileName), nil, 0o644)).To(Succeed())
				return nil
			}
			DeferCleanup(func() { runVerifyFixture = prev })

			report, err := Run(RunOptions{FixturesRoot: root, ArtifactsDir: artifacts, Timeout: time.Second})

			Expect(err).NotTo(HaveOccurred())
			Expect(report.Failed).To(BeFalse())
			Expect(report.Results[0].Unasserted).To(ConsistOf(
				"output expectation missing",
				"decisions expectation missing",
				"dispatch expectation missing",
				"exit-code expectation missing",
			))
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

		It("sorts benchmark results by tool and case before returning and writing report json", func() {
			writeFixtureFile(filepath.Join(root, "zeta", "beta"), "command.yaml", "argv: [\"zeta\"]\n")
			writeFixtureFile(filepath.Join(root, "zeta", "beta"), "stdout.txt", "00000|line\n")
			writeFixtureFile(filepath.Join(root, "zeta", "beta"), "output.txt", "ok\n")
			writeFixtureFile(filepath.Join(root, "alpha", "gamma"), "command.yaml", "argv: [\"alpha\"]\n")
			writeFixtureFile(filepath.Join(root, "alpha", "gamma"), "stdout.txt", "00000|line\n")
			writeFixtureFile(filepath.Join(root, "alpha", "gamma"), "output.txt", "ok\n")
			writeFixtureFile(filepath.Join(root, "alpha", "beta"), "command.yaml", "argv: [\"alpha\"]\n")
			writeFixtureFile(filepath.Join(root, "alpha", "beta"), "stdout.txt", "00000|line\n")
			writeFixtureFile(filepath.Join(root, "alpha", "beta"), "output.txt", "ok\n")

			prev := runVerifyFixture
			runVerifyFixture = func(_ string, dir string, _ time.Duration) error {
				Expect(os.WriteFile(filepath.Join(dir, "verify-output.txt"), []byte("ok\n"), 0o644)).To(Succeed())
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
			Expect(report.Results).To(HaveLen(3))
			Expect([]string{
				report.Results[0].Tool + "/" + report.Results[0].Case,
				report.Results[1].Tool + "/" + report.Results[1].Case,
				report.Results[2].Tool + "/" + report.Results[2].Case,
			}).To(Equal([]string{"alpha/beta", "alpha/gamma", "zeta/beta"}))

			written, err := readRunReport(filepath.Join(artifacts, "report.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect([]string{
				written.Results[0].Tool + "/" + written.Results[0].Case,
				written.Results[1].Tool + "/" + written.Results[1].Case,
				written.Results[2].Tool + "/" + written.Results[2].Case,
			}).To(Equal([]string{"alpha/beta", "alpha/gamma", "zeta/beta"}))
		})

		It("returns the accumulated sorted report when writing report json fails", func() {
			writeFixtureFile(filepath.Join(root, "zeta", "beta"), "command.yaml", "argv: [\"zeta\"]\n")
			writeFixtureFile(filepath.Join(root, "zeta", "beta"), "stdout.txt", "00000|line\n")
			writeFixtureFile(filepath.Join(root, "zeta", "beta"), "output.txt", "ok\n")
			writeFixtureFile(filepath.Join(root, "alpha", "beta"), "command.yaml", "argv: [\"alpha\"]\n")
			writeFixtureFile(filepath.Join(root, "alpha", "beta"), "stdout.txt", "00000|line\n")
			writeFixtureFile(filepath.Join(root, "alpha", "beta"), "output.txt", "ok\n")
			Expect(os.Mkdir(filepath.Join(artifacts, "report.json"), 0o755)).To(Succeed())

			prev := runVerifyFixture
			runVerifyFixture = func(_ string, dir string, _ time.Duration) error {
				Expect(os.WriteFile(filepath.Join(dir, "verify-output.txt"), []byte("ok\n"), 0o644)).To(Succeed())
				return nil
			}
			DeferCleanup(func() { runVerifyFixture = prev })

			report, err := Run(RunOptions{
				FixturesRoot: root,
				ArtifactsDir: artifacts,
				ProxyBinary:  "ccp",
				Timeout:      time.Second,
			})

			Expect(err).To(HaveOccurred())
			Expect(report.Results).To(HaveLen(2))
			Expect([]string{
				report.Results[0].Tool + "/" + report.Results[0].Case,
				report.Results[1].Tool + "/" + report.Results[1].Case,
			}).To(Equal([]string{"alpha/beta", "zeta/beta"}))
		})

		It("warns when token compaction ratio drops from the previous report", func() {
			fixtureDir := filepath.Join(root, "grep", "recursive-match")
			writeFixtureFile(fixtureDir, "command.yaml", "argv: [\"grep\", \"-r\", \"needle\", \"./internal\"]\n")
			writeFixtureFile(fixtureDir, "stdout.txt", "00000|match one\n00001|match two\n")
			writeFixtureFile(fixtureDir, "output.txt", "grouped output\n")

			prev := runVerifyFixture
			runVerifyFixture = func(_ string, dir string, _ time.Duration) error {
				Expect(os.WriteFile(filepath.Join(dir, "verify-output.txt"), []byte("grouped output\n"), 0o644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(dir, "verify-decisions.txt"), []byte("<keep>    | match one\n"), 0o644)).To(Succeed())
				return nil
			}
			DeferCleanup(func() { runVerifyFixture = prev })

			previousReportPath := filepath.Join(GinkgoT().TempDir(), "report.json")
			Expect(writeReportJSON(filepath.Dir(previousReportPath), RunReport{
				Results: []CaseResult{{
					Tool:                 "grep",
					Case:                 "recursive-match",
					InputHash:            fixtureInputHash(replay.CommandSpec{Argv: []string{"grep", "-r", "needle", "./internal"}}, []replay.Event{{Sequence: 0, Line: "match one\n"}, {Sequence: 1, Line: "match two\n"}}),
					TokenCompactionRatio: 10,
				}},
			})).To(Succeed())

			report, err := Run(RunOptions{
				FixturesRoot:   root,
				ArtifactsDir:   artifacts,
				ProxyBinary:    "ccp",
				Timeout:        time.Second,
				PreviousReport: previousReportPath,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(report.Results).To(HaveLen(1))
			Expect(report.Results[0].Warnings).To(ContainElement("token compaction ratio dropped from 10.00 to 1.25"))
			Expect(report.Results[0].Success).To(BeTrue())
			Expect(report.Failed).To(BeFalse())
		})

		It("skips compaction-drop comparison for legacy previous reports without input hash", func() {
			fixtureDir := filepath.Join(root, "grep", "recursive-match")
			writeFixtureFile(fixtureDir, "command.yaml", "argv: [\"grep\", \"-r\", \"needle\", \"./internal\"]\n")
			writeFixtureFile(fixtureDir, "stdout.txt", "00000|match one\n00001|match two\n")
			writeFixtureFile(fixtureDir, "output.txt", "grouped output\n")

			prev := runVerifyFixture
			runVerifyFixture = func(_ string, dir string, _ time.Duration) error {
				Expect(os.WriteFile(filepath.Join(dir, "verify-output.txt"), []byte("grouped output\n"), 0o644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(dir, "verify-decisions.txt"), []byte("<keep>    | match one\n"), 0o644)).To(Succeed())
				return nil
			}
			DeferCleanup(func() { runVerifyFixture = prev })

			previousReportPath := filepath.Join(GinkgoT().TempDir(), "report.json")
			Expect(writeReportJSON(filepath.Dir(previousReportPath), RunReport{
				Results: []CaseResult{{
					Tool:                 "grep",
					Case:                 "recursive-match",
					TokenCompactionRatio: 10,
				}},
			})).To(Succeed())

			report, err := Run(RunOptions{
				FixturesRoot:   root,
				ArtifactsDir:   artifacts,
				ProxyBinary:    "ccp",
				Timeout:        time.Second,
				PreviousReport: previousReportPath,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(report.Results).To(HaveLen(1))
			Expect(report.Results[0].Warnings).To(BeEmpty())
			Expect(report.Results[0].Success).To(BeTrue())
			Expect(report.Failed).To(BeFalse())
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

		DescribeTable("classifies summary helpers deterministically",
			func(result CaseResult, input string, expectedStatus, expectedSanitized string) {
				Expect(summaryStatusCell(result)).To(Equal(expectedStatus))
				Expect(sanitizeSummaryCell(input)).To(Equal(expectedSanitized))
			},
			Entry("uses red status for failed cases and sanitizes markdown delimiters", CaseResult{Success: false}, " bad | row \n", "🔴", "bad / row"),
			Entry("uses yellow status for warning-only compaction drops", CaseResult{Success: true, Warnings: []string{"token compaction ratio dropped from 2.00 to 1.80"}}, "ok", "🟡", "ok"),
			Entry("uses green status for clean successes", CaseResult{Success: true}, " clean ", "🟢", "clean"),
		)

		It("detects whether a summary table header already exists", func() {
			path := filepath.Join(GinkgoT().TempDir(), "summary.md")

			Expect(summaryTableExists(path)).To(BeFalse())
			Expect(os.WriteFile(path, []byte("intro\n"+summaryTableHeaderRow()+"\n"), 0o644)).To(Succeed())
			Expect(summaryTableExists(path)).To(BeTrue())
		})

		It("writes summary rows in sorted tool and case order", func() {
			summaryPath := filepath.Join(GinkgoT().TempDir(), "summary.md")
			prev := os.Getenv("GITHUB_STEP_SUMMARY")
			Expect(os.Setenv("GITHUB_STEP_SUMMARY", summaryPath)).To(Succeed())
			DeferCleanup(func() {
				if prev == "" {
					_ = os.Unsetenv("GITHUB_STEP_SUMMARY")
					return
				}
				_ = os.Setenv("GITHUB_STEP_SUMMARY", prev)
			})

			Expect(WriteSummary(RunReport{
				Results: []CaseResult{
					{Tool: "zeta", Case: "beta", Command: "zeta", NativeTokens: 1, ProxyTokens: 1, Success: true},
					{Tool: "alpha", Case: "gamma", Command: "alpha gamma", NativeTokens: 2, ProxyTokens: 1, Success: true},
					{Tool: "alpha", Case: "beta", Command: "alpha beta", NativeTokens: 3, ProxyTokens: 1, Success: true},
				},
			})).To(Succeed())

			body, err := os.ReadFile(summaryPath)
			Expect(err).NotTo(HaveOccurred())
			text := string(body)
			first := strings.Index(text, "| 🟢 | alpha/beta |")
			second := strings.Index(text, "| 🟢 | alpha/gamma |")
			third := strings.Index(text, "| 🟢 | zeta/beta |")
			Expect(first).To(BeNumerically(">=", 0))
			Expect(second).To(BeNumerically(">", first))
			Expect(third).To(BeNumerically(">", second))
		})

		It("writes markdown summary to GITHUB_STEP_SUMMARY when set", func() {
			summaryPath := filepath.Join(GinkgoT().TempDir(), "summary.md")
			prev := os.Getenv("GITHUB_STEP_SUMMARY")
			Expect(os.Setenv("GITHUB_STEP_SUMMARY", summaryPath)).To(Succeed())
			DeferCleanup(func() {
				if prev == "" {
					_ = os.Unsetenv("GITHUB_STEP_SUMMARY")
					return
				}
				_ = os.Setenv("GITHUB_STEP_SUMMARY", prev)
			})

			report := RunReport{
				Generated: time.Date(2026, time.March, 17, 12, 0, 0, 0, time.UTC),
				Results: []CaseResult{{
					Tool:         "grep",
					Case:         "recursive-match",
					Command:      "grep -r needle ./internal",
					NativeTokens: 10,
					ProxyTokens:  4,
					Success:      true,
				}},
			}

			Expect(WriteSummary(report)).To(Succeed())

			body, err := os.ReadFile(summaryPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("| Status | Case | Command | Native tokens | Proxy tokens | Token savings % | Notes |"))
			Expect(string(body)).To(ContainSubstring("| 🟢 | grep/recursive-match | `grep -r needle ./internal` | 10 | 4 | 60.00 |  |"))
		})

		It("appends multiple invocations into a single benchmark table", func() {
			summaryPath := filepath.Join(GinkgoT().TempDir(), "summary.md")
			prev := os.Getenv("GITHUB_STEP_SUMMARY")
			Expect(os.Setenv("GITHUB_STEP_SUMMARY", summaryPath)).To(Succeed())
			DeferCleanup(func() {
				if prev == "" {
					_ = os.Unsetenv("GITHUB_STEP_SUMMARY")
					return
				}
				_ = os.Setenv("GITHUB_STEP_SUMMARY", prev)
			})

			Expect(WriteSummary(RunReport{
				Results: []CaseResult{{
					Tool:         "grep",
					Case:         "recursive-match",
					Command:      "grep -r needle ./internal",
					NativeTokens: 10,
					ProxyTokens:  4,
					Success:      true,
				}},
			})).To(Succeed())
			Expect(WriteSummary(RunReport{
				Results: []CaseResult{{
					Tool:         "grep",
					Case:         "no-match",
					Command:      "grep needle missing",
					NativeTokens: 3,
					ProxyTokens:  0,
					Success:      false,
					Warnings:     []string{"output mismatch"},
				}},
			})).To(Succeed())

			body, err := os.ReadFile(summaryPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.Count(string(body), "| Status | Case | Command | Native tokens | Proxy tokens | Token savings % | Notes |")).To(Equal(1))
			Expect(string(body)).To(ContainSubstring("| 🟢 | grep/recursive-match | `grep -r needle ./internal` | 10 | 4 | 60.00 |  |"))
			Expect(string(body)).To(ContainSubstring("| 🔴 | grep/no-match | `grep needle missing` | 3 | 0 | 100.00 | output mismatch |"))
		})

		It("prints markdown summary to stdout when GITHUB_STEP_SUMMARY is unset", func() {
			prevEnv := os.Getenv("GITHUB_STEP_SUMMARY")
			Expect(os.Unsetenv("GITHUB_STEP_SUMMARY")).To(Succeed())
			DeferCleanup(func() {
				if prevEnv == "" {
					return
				}
				_ = os.Setenv("GITHUB_STEP_SUMMARY", prevEnv)
			})

			stdoutReader, stdoutWriter, err := os.Pipe()
			Expect(err).NotTo(HaveOccurred())
			prevStdout := os.Stdout
			os.Stdout = stdoutWriter
			DeferCleanup(func() { os.Stdout = prevStdout })

			output := make(chan string, 1)
			go func() {
				var buf bytes.Buffer
				_, _ = io.Copy(&buf, stdoutReader)
				output <- buf.String()
			}()

			report := RunReport{
				Generated: time.Date(2026, time.March, 17, 12, 0, 0, 0, time.UTC),
				Results: []CaseResult{{
					Tool:         "grep",
					Case:         "no-match",
					Command:      "grep needle missing",
					NativeTokens: 0,
					ProxyTokens:  0,
					Success:      false,
					Warnings:     []string{"output mismatch"},
				}},
			}

			Expect(WriteSummary(report)).To(Succeed())
			Expect(stdoutWriter.Close()).To(Succeed())
			Expect(<-output).To(ContainSubstring("| 🔴 | grep/no-match | `grep needle missing` | 0 | 0 | 0.00 | output mismatch |"))
		})

		It("renders warning-only rows with a yellow status", func() {
			summaryPath := filepath.Join(GinkgoT().TempDir(), "summary.md")
			prev := os.Getenv("GITHUB_STEP_SUMMARY")
			Expect(os.Setenv("GITHUB_STEP_SUMMARY", summaryPath)).To(Succeed())
			DeferCleanup(func() {
				if prev == "" {
					_ = os.Unsetenv("GITHUB_STEP_SUMMARY")
					return
				}
				_ = os.Setenv("GITHUB_STEP_SUMMARY", prev)
			})

			Expect(WriteSummary(RunReport{
				Results: []CaseResult{{
					Tool:                 "grep",
					Case:                 "warning-only",
					Command:              "grep warning",
					NativeTokens:         10,
					ProxyTokens:          9,
					Success:              true,
					Warnings:             []string{"token compaction ratio dropped from 10.00 to 1.11"},
					TokenCompactionRatio: 1.11,
				}},
			})).To(Succeed())

			body, err := os.ReadFile(summaryPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("| 🟡 | grep/warning-only | `grep warning` | 10 | 9 | 10.00 | token compaction ratio dropped from 10.00 to 1.11 |"))
		})

		DescribeTable("loads previous results",
			func(contents string, useMissingPath bool, expectKey bool, matcher OmegaMatcher) {
				path := ""
				if useMissingPath {
					path = filepath.Join(GinkgoT().TempDir(), "missing", "report.json")
				} else if contents != "" {
					path = filepath.Join(GinkgoT().TempDir(), "report.json")
					Expect(os.WriteFile(path, []byte(contents), 0o644)).To(Succeed())
				}
				results, err := loadPreviousResults(path)

				Expect(err).To(matcher)
				if err == nil {
					if expectKey {
						Expect(results).To(HaveKey(comparisonKey("grep", "recursive-match")))
					} else {
						Expect(results).To(BeEmpty())
					}
				}
			},
			Entry("ignores blank path", "", false, false, Succeed()),
			Entry("ignores missing file", "", true, false, Succeed()),
			Entry("loads prior case results", `{"results":[{"tool":"grep","case":"recursive-match","token_compaction_ratio":2.5}]}`, false, true, Succeed()),
			Entry("reports invalid JSON", "{", false, false, MatchError(ContainSubstring("read previous report: parse report json:"))),
		)

		DescribeTable("handles compaction-ratio comparisons",
			func(current *CaseResult, previous CaseResult, expectedWarnings []string, expectedSuccess bool) {
				maybeWarnCompactionDrop(current, previous)
				Expect(current.Warnings).To(Equal(expectedWarnings))
				Expect(current.Success).To(Equal(expectedSuccess))
			},
			Entry("ignores missing previous ratio", &CaseResult{InputHash: "same", Success: true}, CaseResult{InputHash: "same"}, nil, true),
			Entry("ignores legacy previous reports without input hash", &CaseResult{InputHash: "current", TokenCompactionRatio: 1.25, Success: true}, CaseResult{TokenCompactionRatio: 10}, nil, true),
			Entry("ignores changed input hash", &CaseResult{InputHash: "current", TokenCompactionRatio: 1.25, Success: true}, CaseResult{InputHash: "previous", TokenCompactionRatio: 10}, nil, true),
			Entry("ignores non-positive previous ratios", &CaseResult{InputHash: "same", TokenCompactionRatio: 1.25, Success: true}, CaseResult{InputHash: "same", TokenCompactionRatio: 0}, nil, true),
			Entry("ignores negative previous ratios", &CaseResult{InputHash: "same", TokenCompactionRatio: 1.25, Success: true}, CaseResult{InputHash: "same", TokenCompactionRatio: -1}, nil, true),
			Entry("does not warn at the exact five percent threshold", &CaseResult{InputHash: "same", TokenCompactionRatio: 1.9, Success: true}, CaseResult{InputHash: "same", TokenCompactionRatio: 2}, nil, true),
			Entry("ignores stable ratios", &CaseResult{InputHash: "same", TokenCompactionRatio: 1.9, Success: true}, CaseResult{InputHash: "same", TokenCompactionRatio: 2}, nil, true),
			Entry("warns once the drop moves just past the five percent threshold", &CaseResult{InputHash: "same", TokenCompactionRatio: 1.89, Success: true}, CaseResult{InputHash: "same", TokenCompactionRatio: 2}, []string{"token compaction ratio dropped from 2.00 to 1.89"}, true),
			Entry("warns on material drop", &CaseResult{InputHash: "same", TokenCompactionRatio: 1.25, Success: true}, CaseResult{InputHash: "same", TokenCompactionRatio: 10}, []string{"token compaction ratio dropped from 10.00 to 1.25"}, true),
		)

		DescribeTable("computes token compaction ratio",
			func(nativeTokens, proxyTokens int, expected float64) {
				Expect(tokenCompactionRatio(nativeTokens, proxyTokens)).To(Equal(expected))
			},
			Entry("uses native to proxy ratio", 10, 4, 2.5),
			Entry("treats zero proxy tokens as best-case improvement", 10, 0, 10.0),
			Entry("returns a neutral ratio when both token counts are zero", 0, 0, 1.0),
		)

		DescribeTable("normalizing token savings percentages",
			func(result CaseResult, expected float64) {
				Expect(tokenSavingsPct(result)).To(BeNumerically("~", expected, 1e-12))
			},
			Entry("returns zero for no native tokens", CaseResult{NativeTokens: 0, ProxyTokens: 10}, 0.0),
			Entry("rounds tiny regressions to zero", CaseResult{NativeTokens: 100, ProxyTokens: 103}, 0.0),
			Entry("rounds tiny improvements to zero", CaseResult{NativeTokens: 100, ProxyTokens: 96}, 0.0),
			Entry("keeps exact five percent savings", CaseResult{NativeTokens: 100, ProxyTokens: 95}, 5.0),
			Entry("keeps exact five percent regressions", CaseResult{NativeTokens: 100, ProxyTokens: 105}, -5.0),
			Entry("keeps improvements just past the flat threshold", CaseResult{NativeTokens: 100, ProxyTokens: 94}, 6.0),
			Entry("keeps regressions just past the flat threshold", CaseResult{NativeTokens: 100, ProxyTokens: 106}, -6.0),
			Entry("preserves meaningful regressions", CaseResult{NativeTokens: 100, ProxyTokens: 110}, -10.000000000000009),
		)

		It("hashes fixture input from command and replayed streams", func() {
			command := replay.CommandSpec{Argv: []string{"grep", "-r", "needle", "."}, ExitCode: 2}
			events := []replay.Event{
				{Sequence: 0, Stream: contracts.StreamStdout, Line: "match one\n"},
				{Sequence: 1, Stream: contracts.StreamStdout, Line: "match two\n"},
			}

			hashA := fixtureInputHash(command, events)
			hashB := fixtureInputHash(command, events)
			hashC := fixtureInputHash(replay.CommandSpec{Argv: []string{"grep", "-r", "needle", "./internal"}, ExitCode: 2}, events)

			Expect(hashA).To(HaveLen(64))
			Expect(hashB).To(Equal(hashA))
			Expect(hashC).NotTo(Equal(hashA))
		})
	})

	Describe("file helpers", func() {
		It("persists benchmark metric bytes from token counts", func() {
			artifactDir := GinkgoT().TempDir()

			Expect(appendCaseMetrics(artifactDir, []string{"grep", "-r", "needle", "."}, 7, 3)).To(Succeed())

			history, err := metrics.QueryHistory(filepath.Join(artifactDir, ".ccp", "gain.db"), metrics.QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(HaveLen(1))
			Expect(history[0].Command).To(Equal("grep -r needle ."))
			Expect(history[0].Tool).To(Equal("grep"))
			Expect(history[0].DispatchKey).To(Equal("grep"))
			Expect(history[0].RawBytes).To(Equal(int64(28)))
			Expect(history[0].KeptBytes).To(Equal(int64(12)))
		})

		It("copies fixture inputs when present", func() {
			srcDir := GinkgoT().TempDir()
			dstDir := GinkgoT().TempDir()
			fixture := replayFixture(srcDir)

			Expect(copyFixtureInputs(fixture, dstDir)).To(Succeed())
			Expect(filepath.Join(dstDir, "command.yaml")).To(BeAnExistingFile())
			Expect(filepath.Join(dstDir, "stdout.txt")).To(BeAnExistingFile())
			Expect(filepath.Join(dstDir, "stderr.txt")).To(BeAnExistingFile())
			Expect(filepath.Join(dstDir, "output.txt")).To(BeAnExistingFile())
		})

		It("copies output-only fixtures for verify replay", func() {
			srcDir := GinkgoT().TempDir()
			dstDir := GinkgoT().TempDir()
			writeFixtureFile(srcDir, "command.yaml", "argv: [\"grep\"]\n")
			writeFixtureFile(srcDir, "output.txt", "")

			fixture, err := replay.LoadFixture(srcDir)
			Expect(err).NotTo(HaveOccurred())

			Expect(copyFixtureInputs(fixture, dstDir)).To(Succeed())
			Expect(filepath.Join(dstDir, "command.yaml")).To(BeAnExistingFile())
			Expect(filepath.Join(dstDir, "output.txt")).To(BeAnExistingFile())
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

		It("reports appendSummaryToFile errors for invalid destinations", func() {
			dir := GinkgoT().TempDir()

			err := appendSummaryToFile(dir, "| row |\n")

			Expect(err).To(HaveOccurred())
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
	writeFixtureFile(dir, "output.txt", "out\n")
	return replay.Fixture{
		CommandPath: filepath.Join(dir, "command.yaml"),
		StdoutPath:  filepath.Join(dir, "stdout.txt"),
		StderrPath:  filepath.Join(dir, "stderr.txt"),
		OutputPath:  filepath.Join(dir, "output.txt"),
	}
}
