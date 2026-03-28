package lifecycle

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/metrics"
	"go-command-compression-proxy/internal/workspaces"
)

const (
	flagFormat = "--format"
	flagPeriod = "--period"
	flagSince  = "--since"
	flagTable  = "--table"
	flagGlobal = "--global"
	flagLimit  = "--limit"
)

var _ = Describe("RunGain", func() {
	var (
		tmpDir      string
		path        string
		defaultSeed []metrics.RunMetric
		runGain     func(args ...string) string
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		path = filepath.Join(tmpDir, "gain.db")
		defaultSeed = []metrics.RunMetric{
			{
				Timestamp:   time.Now().UTC().Add(-2 * time.Hour),
				Tool:        "go",
				Command:     "go test ./...",
				RawBytes:    1200,
				KeptBytes:   400,
				ExitCode:    0,
				Passthrough: false,
			},
			{
				Timestamp:   time.Now().UTC().Add(-1 * time.Hour),
				Tool:        "git",
				Command:     "git push origin main",
				RawBytes:    500,
				KeptBytes:   500,
				ExitCode:    0,
				Passthrough: true,
			},
		}
		appendGainMetrics(path, defaultSeed)
		runGain = func(args ...string) string {
			orig := os.Stdout
			r, w, err := os.Pipe()
			Expect(err).NotTo(HaveOccurred())
			os.Stdout = w
			DeferCleanup(func() { os.Stdout = orig })

			Expect(RunGain(args, path)).To(Succeed())
			Expect(w.Close()).To(Succeed())

			var buf bytes.Buffer
			_, err = io.Copy(&buf, r)
			Expect(err).NotTo(HaveOccurred())
			Expect(r.Close()).To(Succeed())
			return buf.String()
		}
	})

	Context("when rendering summary output", func() {
		It("renders gain JSON", func() {
			out := runGain("--json")
			Expect(out).To(ContainSubstring(`"dataset": "summary"`))
			Expect(out).To(ContainSubstring(`"total"`))
		})

		It("renders default gain output as text", func() {
			out := runGain()
			plain := engine.StripANSI(out)
			Expect(out).To(ContainSubstring("\x1b["))
			Expect(plain).To(ContainSubstring("2 cmds · 425 → 225 tokens (47.1% saved)"))
			Expect(plain).To(ContainSubstring("Wins  : go (200 / 67%)"))
			Expect(plain).To(ContainSubstring("Drag  : git (1 cmds)"))
			Expect(plain).To(ContainSubstring("Trend : insufficient data"))
		})

		It("renders the compact gain table", func() {
			out := runGain(flagTable)
			Expect(out).To(ContainSubstring("2 cmds · 425 → 225 tokens (47.1% saved)"))
			Expect(out).NotTo(ContainSubstring("\x1b["))
			Expect(out).To(ContainSubstring("showing 2 of 2 tools"))
			Expect(out).To(ContainSubstring("+------+"))
			Expect(out).To(ContainSubstring("TOOL"))
			Expect(out).To(ContainSubstring("NATIVE"))
			Expect(out).To(ContainSubstring("PROXIED"))
			Expect(out).To(ContainSubstring("SAVINGS"))
			Expect(out).NotTo(ContainSubstring("TOTAL"))
			Expect(out).NotTo(ContainSubstring("COMMAND"))
			Expect(out).To(ContainSubstring("go"))
			Expect(out).To(ContainSubstring("git"))
		})

		It("aggregates registered workspaces for global gain output", func() {
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)

			repoOne := filepath.Join(tmpDir, "repo-one")
			repoTwo := filepath.Join(tmpDir, "repo-two")
			appendGlobalWorkspaceMetrics(home, repoOne, []metrics.RunMetric{
				{Timestamp: time.Now().UTC().Add(-2 * time.Hour), Tool: "go", Command: "go test ./...", RawBytes: 1200, KeptBytes: 400},
			})
			appendGlobalWorkspaceMetrics(home, repoTwo, []metrics.RunMetric{
				{Timestamp: time.Now().UTC().Add(-time.Hour), Tool: "git", Command: "git status", RawBytes: 500, KeptBytes: 500},
			})

			out := runGain(flagGlobal)
			plain := engine.StripANSI(out)
			Expect(out).To(ContainSubstring("\x1b["))
			Expect(plain).To(ContainSubstring("4 cmds · 850 → 450 tokens (47.1% saved) [global]"))
			Expect(plain).To(ContainSubstring("Wins  : go (400 / 67%)"))
			Expect(plain).To(ContainSubstring("Drag  : git (2 cmds)"))
			Expect(plain).To(ContainSubstring("go"))
			Expect(plain).To(ContainSubstring("git"))
		})

		It("compares adjacent global week windows for compact trends", func() {
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)
			Expect(os.Remove(path)).To(Succeed())

			nowUTC := time.Now().UTC()
			referenceDay := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 12, 0, 0, 0, time.UTC)
			repo := filepath.Join(tmpDir, "repo-trend")
			appendGlobalWorkspaceMetrics(home, repo, []metrics.RunMetric{
				{
					Timestamp: referenceDay.AddDate(0, 0, -10),
					Tool:      "go",
					Command:   "go test ./...",
					RawBytes:  400,
					KeptBytes: 200,
				},
				{
					Timestamp: referenceDay.AddDate(0, 0, -2),
					Tool:      "go",
					Command:   "go test ./...",
					RawBytes:  400,
					KeptBytes: 100,
				},
			})

			out := runGain(flagGlobal)
			plain := engine.StripANSI(out)
			Expect(out).To(ContainSubstring("\x1b["))
			Expect(plain).To(ContainSubstring("Trend : \u2191 +25.0 pts week over week (50.0% \u2192 75.0%)"))
		})

		It("weights global trend windows by actual volume instead of averaging bucket percentages", func() {
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)
			Expect(os.Remove(path)).To(Succeed())

			nowUTC := time.Now().UTC()
			referenceDay := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 12, 0, 0, 0, time.UTC)
			repo := filepath.Join(tmpDir, "repo-weighted-trend")
			appendGlobalWorkspaceMetrics(home, repo, []metrics.RunMetric{
				{
					Timestamp: referenceDay.AddDate(0, 0, -13),
					Tool:      "go",
					Command:   "go test ./...",
					RawBytes:  400,
					KeptBytes: 200,
				},
				{
					Timestamp: referenceDay.AddDate(0, 0, -12),
					Tool:      "go",
					Command:   "go test ./...",
					RawBytes:  4,
					KeptBytes: 0,
				},
				{
					Timestamp: referenceDay.AddDate(0, 0, -6),
					Tool:      "go",
					Command:   "go test ./...",
					RawBytes:  400,
					KeptBytes: 100,
				},
				{
					Timestamp: referenceDay.AddDate(0, 0, -5),
					Tool:      "go",
					Command:   "go test ./...",
					RawBytes:  4,
					KeptBytes: 4,
				},
			})

			out := runGain(flagGlobal)
			plain := engine.StripANSI(out)
			Expect(out).To(ContainSubstring("\x1b["))
			Expect(plain).To(ContainSubstring("Trend : \u2191 +23.8 pts week over week (50.5% \u2192 74.3%)"))
		})

		It("warns when skipping corrupt registered workspaces during global gain aggregation", func() {
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)

			repoOne := filepath.Join(tmpDir, "repo-one")
			appendGlobalWorkspaceMetrics(home, repoOne, []metrics.RunMetric{
				{Timestamp: time.Now().UTC().Add(-2 * time.Hour), Tool: "go", Command: "go test ./...", RawBytes: 1200, KeptBytes: 400},
			})

			corruptRepo := filepath.Join(tmpDir, "corrupt-repo")
			corruptMetricsPath := filepath.Join(corruptRepo, ".ccp", "gain.db")
			Expect(os.MkdirAll(filepath.Dir(corruptMetricsPath), 0o755)).To(Succeed())
			Expect(os.WriteFile(corruptMetricsPath, []byte("not-a-bolt-db"), 0o644)).To(Succeed())
			Expect(workspaces.UpsertPath(workspaces.PathForHome(home), corruptRepo, corruptMetricsPath)).To(Succeed())

			var stdout string
			stderr, err := captureStderrOutput(func() error {
				var runErr error
				stdout, runErr = captureStdout(func() error {
					return RunGain([]string{flagGlobal, flagFormat, "json"}, path)
				})
				return runErr
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(stdout).To(ContainSubstring(`"dataset": "summary"`))
			Expect(stdout).To(ContainSubstring(`"go test ./..."`))
			Expect(stderr).To(ContainSubstring("ccp gain --global: warning: skipped workspace"))
			Expect(stderr).To(ContainSubstring(corruptRepo))
			Expect(stderr).To(ContainSubstring(corruptMetricsPath))
			Expect(stderr).To(ContainSubstring("results exclude 1 workspace(s)"))
		})

		It("includes the current repo legacy gain database even when the registry is empty", func() {
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)

			repoRoot := filepath.Join(tmpDir, "legacy-repo")
			Expect(os.MkdirAll(repoRoot, 0o755)).To(Succeed())
			repoMetricsPath := filepath.Join(repoRoot, ".ccp", "gain.db")
			appendGainMetrics(repoMetricsPath, []metrics.RunMetric{
				{Timestamp: time.Now().UTC().Add(-time.Hour), Tool: "go", Command: "go test ./...", RawBytes: 1200, KeptBytes: 400},
			})

			var out string
			Expect(runInDir(repoRoot, func() error {
				var err error
				out, err = captureStdout(func() error {
					return RunGain([]string{flagGlobal, flagFormat, "json"}, repoMetricsPath)
				})
				return err
			})).To(Succeed())

			Expect(out).To(ContainSubstring(`"go test ./..."`))
			entries, err := workspaces.ListPath(workspaces.PathForHome(home))
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(resolvedPath(entries[0].CWD)).To(Equal(resolvedPath(repoRoot)))
			Expect(entries[0].MetricsPath).To(Equal(repoMetricsPath))
		})

		It("does not double count the current repo when it is already registered", func() {
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)

			repoRoot := filepath.Join(tmpDir, "current-repo")
			Expect(os.MkdirAll(repoRoot, 0o755)).To(Succeed())
			repoMetricsPath := filepath.Join(repoRoot, ".ccp", "gain.db")
			appendGainMetrics(repoMetricsPath, []metrics.RunMetric{
				{Timestamp: time.Now().UTC().Add(-time.Hour), Tool: "go", Command: "go test ./...", RawBytes: 1200, KeptBytes: 400},
			})
			Expect(workspaces.UpsertPath(workspaces.PathForHome(home), repoRoot, repoMetricsPath)).To(Succeed())

			var out string
			Expect(runInDir(repoRoot, func() error {
				var err error
				out, err = captureStdout(func() error {
					return RunGain([]string{flagGlobal, flagFormat, "json"}, repoMetricsPath)
				})
				return err
			})).To(Succeed())

			var env summaryEnvelope
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env.Total.Commands).To(Equal(int64(1)))
			Expect(env.Rows).To(HaveLen(1))
		})

		It("falls back to current repo metrics when the workspace registry is unreadable", func() {
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)

			registryPath := workspaces.PathForHome(home)
			Expect(os.MkdirAll(filepath.Dir(registryPath), 0o755)).To(Succeed())
			Expect(os.WriteFile(registryPath, []byte("not-a-bolt-db"), 0o644)).To(Succeed())

			repoRoot := filepath.Join(tmpDir, "broken-registry-repo")
			Expect(os.MkdirAll(repoRoot, 0o755)).To(Succeed())
			repoMetricsPath := filepath.Join(repoRoot, ".ccp", "gain.db")
			appendGainMetrics(repoMetricsPath, []metrics.RunMetric{
				{Timestamp: time.Now().UTC().Add(-time.Hour), Tool: "go", Command: "go test ./...", RawBytes: 1200, KeptBytes: 400},
			})

			var out string
			Expect(runInDir(repoRoot, func() error {
				var err error
				out, err = captureStdout(func() error {
					return RunGain([]string{flagGlobal, flagFormat, "json"}, repoMetricsPath)
				})
				return err
			})).To(Succeed())

			Expect(out).To(ContainSubstring(`"go test ./..."`))
		})

		It("renders global period summaries and datasets", func() {
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)

			nowUTC := time.Now().UTC()
			now := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)
			repoOne := filepath.Join(tmpDir, "repo-one")
			repoTwo := filepath.Join(tmpDir, "repo-two")
			appendGlobalWorkspaceMetrics(home, repoOne, []metrics.RunMetric{
				{Timestamp: now.Add(-6*24*time.Hour + 9*time.Hour), Tool: "go", Command: "go test ./...", RawBytes: 1200, KeptBytes: 200},
				{Timestamp: now.Add(-4*24*time.Hour + 9*time.Hour), Tool: "git", Command: "git status", RawBytes: 400, KeptBytes: 400},
			})
			appendGlobalWorkspaceMetrics(home, repoTwo, []metrics.RunMetric{
				{Timestamp: now.Add(-5*24*time.Hour + 9*time.Hour), Tool: "grep", Command: "grep -r needle .", RawBytes: 800, KeptBytes: 200},
				{Timestamp: now.Add(-4*24*time.Hour + 10*time.Hour), Tool: "sed", Command: "sed -n 1,20p file", RawBytes: 100, KeptBytes: 100},
			})

			textOut := runGain(flagGlobal, flagPeriod, "week")
			plainTextOut := engine.StripANSI(textOut)
			Expect(plainTextOut).To(ContainSubstring("[period=week global]"))
			Expect(plainTextOut).To(ContainSubstring("Wins  :"))
			Expect(plainTextOut).To(ContainSubstring("Drag  :"))
			Expect(plainTextOut).To(ContainSubstring("Trend :"))

			jsonOut := runGain(flagGlobal, flagPeriod, "day", flagFormat, "json")
			Expect(jsonOut).To(ContainSubstring(`"dataset": "period"`))
			Expect(jsonOut).To(ContainSubstring(`"bucket"`))

			csvOut := runGain(flagGlobal, flagPeriod, "day", flagFormat, "csv")
			Expect(csvOut).To(ContainSubstring("dataset,period,since,tool_filter,failed_filter,bucket"))
			Expect(csvOut).To(ContainSubstring("period,day"))
		})
	})

	Context("when formatting aggregated values", func() {
		BeforeEach(func() {
			Expect(os.Remove(path)).To(Succeed())
			now := time.Now().UTC()
			appendGainMetrics(path, []metrics.RunMetric{
				{Timestamp: now.Add(-4 * time.Hour), Tool: "gradle", Command: "./gradlew test", RawBytes: 20_000_000, KeptBytes: 100_000, ExitCode: 0},
				{Timestamp: now.Add(-3 * time.Hour), Tool: "jar", Command: "jar tf app.jar", RawBytes: 8_000, KeptBytes: 8_000, ExitCode: 0},
			})
		})

		It("formats grouped numbers and zero-savings text in default output", func() {
			out := runGain()
			plain := engine.StripANSI(out)
			Expect(plain).To(ContainSubstring("5,002,000"))
			Expect(plain).To(ContainSubstring("27,000"))
			Expect(plain).To(ContainSubstring("Wins  : gradle (5m / 100%)"))
			Expect(plain).To(ContainSubstring("Drag  : jar (1 cmds)"))
		})

		It("formats grouped numbers in the table output", func() {
			out := runGain(flagTable)
			Expect(out).To(ContainSubstring("5,002,000"))
			Expect(out).To(ContainSubstring("27,000"))
		})
	})

	Context("when rendering the gain table", func() {
		It("orders tied rows by native bytes and tool name", func() {
			Expect(os.Remove(path)).To(Succeed())
			now := time.Now().UTC()
			appendGainMetrics(path, []metrics.RunMetric{
				{Timestamp: now.Add(-4 * time.Minute), Tool: "alpha", Command: "alpha cmd", RawBytes: 200, KeptBytes: 100},
				{Timestamp: now.Add(-3 * time.Minute), Tool: "zeta", Command: "zeta cmd", RawBytes: 200, KeptBytes: 100},
				{Timestamp: now.Add(-2 * time.Minute), Tool: "middle", Command: "middle cmd", RawBytes: 120, KeptBytes: 60},
				{Timestamp: now.Add(-1 * time.Minute), Tool: "zero", Command: "zero cmd", RawBytes: 80, KeptBytes: 80},
			})

			out := runGain(flagTable)

			alphaIdx := strings.Index(out, "alpha")
			zetaIdx := strings.Index(out, "zeta")
			middleIdx := strings.Index(out, "middle")
			zeroIdx := strings.Index(out, "zero")
			Expect(alphaIdx).To(BeNumerically(">=", 0), out)
			Expect(zetaIdx).To(BeNumerically(">=", 0), out)
			Expect(middleIdx).To(BeNumerically(">=", 0), out)
			Expect(zeroIdx).To(BeNumerically(">=", 0), out)
			Expect(alphaIdx).To(BeNumerically("<", middleIdx), out)
			Expect(zetaIdx).To(BeNumerically("<", middleIdx), out)
			Expect(alphaIdx).To(BeNumerically("<", zetaIdx), out)
			Expect(middleIdx).To(BeNumerically("<", zeroIdx), out)
		})

		It("applies the text limit to table output", func() {
			Expect(os.Remove(path)).To(Succeed())
			now := time.Now().UTC()
			for i := range 20 {
				appendGainMetrics(path, []metrics.RunMetric{{
					Timestamp: now.Add(time.Duration(-i) * time.Minute),
					Tool:      fmt.Sprintf("tool-%02d", i),
					Command:   fmt.Sprintf("tool-%02d cmd", i),
					RawBytes:  400,
					KeptBytes: 200,
				}})
			}

			out := runGain(flagTable)
			Expect(out).To(ContainSubstring("showing 15 of 20 tools, use --limit N to see more"))
			Expect(out).To(ContainSubstring("tool-00"))
			Expect(out).NotTo(ContainSubstring("tool-19"))
		})

		It("ignores --limit for non-text gain output", func() {
			jsonOut := runGain(flagFormat, "json", flagLimit, "1")
			var env summaryEnvelope
			Expect(json.Unmarshal([]byte(jsonOut), &env)).To(Succeed())
			Expect(env.Rows).To(HaveLen(2))

			csvOut := runGain(flagFormat, "csv", flagLimit, "1")
			Expect(strings.Count(strings.TrimSpace(csvOut), "\n")).To(Equal(3))
		})
	})

	Context("when rendering period summaries", func() {
		It("renders text, table, and csv period formats", func() {
			csvOut := runGain(flagFormat, "csv")
			Expect(csvOut).To(ContainSubstring("dataset,period,since,tool_filter"))
			Expect(csvOut).To(ContainSubstring("summary"))

			periodText := runGain(flagPeriod, "day")
			plainPeriodText := engine.StripANSI(periodText)
			Expect(plainPeriodText).To(ContainSubstring("[period=day]"))
			Expect(plainPeriodText).To(ContainSubstring("Wins  :"))
			Expect(plainPeriodText).To(ContainSubstring("Drag  :"))
			Expect(plainPeriodText).To(ContainSubstring("Trend :"))

			periodTable := runGain(flagPeriod, "day", flagTable)
			Expect(periodTable).To(ContainSubstring("ccp gain (estimated tokens: 4B/token)"))
			Expect(periodTable).To(ContainSubstring("BUCKET"))
			Expect(periodTable).To(ContainSubstring("period=day"))

			periodCSV := runGain(flagPeriod, "week", flagFormat, "csv")
			Expect(periodCSV).To(ContainSubstring("dataset,period,since,tool_filter,failed_filter,bucket"))
			Expect(periodCSV).To(ContainSubstring("period,week"))
		})

		It("applies the text limit to local period tables", func() {
			Expect(os.Remove(path)).To(Succeed())
			now := time.Now().UTC()
			rows := make([]metrics.RunMetric, 0, 20)
			for i := range 20 {
				rows = append(rows, metrics.RunMetric{
					Timestamp: now.Add(-time.Duration(i) * 24 * time.Hour),
					Tool:      "go",
					Command:   fmt.Sprintf("go test ./pkg/%02d", i),
					RawBytes:  1200,
					KeptBytes: 400,
				})
			}
			appendGainMetrics(path, rows)

			newestBucket := now.Format("2006-01-02")
			oldestBucket := now.Add(-19 * 24 * time.Hour).Format("2006-01-02")

			limitedOut := runGain(flagPeriod, "day", flagTable, flagLimit, "5")
			Expect(limitedOut).To(ContainSubstring("showing 5 of 20 buckets, use --limit N to see more"))
			Expect(limitedOut).To(ContainSubstring(newestBucket))
			Expect(limitedOut).NotTo(ContainSubstring(oldestBucket))

			unlimitedOut := runGain(flagPeriod, "day", flagTable, flagLimit, "0")
			Expect(unlimitedOut).To(ContainSubstring("showing 20 of 20 buckets"))
			Expect(unlimitedOut).NotTo(ContainSubstring(", use --limit N to see more"))
			Expect(unlimitedOut).To(ContainSubstring(oldestBucket))
			Expect(unlimitedOut).To(ContainSubstring(newestBucket))
		})

		It("applies the text limit to global period tables", func() {
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)

			repo := filepath.Join(tmpDir, "repo")
			now := time.Now().UTC()
			rows := make([]metrics.RunMetric, 0, 20)
			for i := range 20 {
				rows = append(rows, metrics.RunMetric{
					Timestamp: now.Add(-time.Duration(i) * 24 * time.Hour),
					Tool:      "go",
					Command:   fmt.Sprintf("go test ./pkg/%02d", i),
					RawBytes:  1200,
					KeptBytes: 400,
				})
			}
			Expect(os.Remove(path)).To(Succeed())
			appendGlobalWorkspaceMetrics(home, repo, rows)

			periodText := runGain(flagGlobal, flagPeriod, "day", flagTable, flagLimit, "5")
			Expect(periodText).To(ContainSubstring("showing 5 of 20 buckets, use --limit N to see more"))
			Expect(periodText).To(ContainSubstring("filters: since=all tool=* failed=false period=day"))
			Expect(periodText).To(ContainSubstring(now.Add(-19 * 24 * time.Hour).Format("2006-01-02")))
			Expect(periodText).NotTo(ContainSubstring(now.Format("2006-01-02")))
		})
	})

	Context("when the gain database is empty", func() {
		BeforeEach(func() {
			Expect(os.Remove(path)).To(Succeed())
		})

		It("handles empty and removable databases", func() {
			Expect(RunGain(nil, path)).To(Succeed())
			err := os.Remove(path)
			Expect(err == nil || errors.Is(err, os.ErrNotExist)).To(BeTrue())
		})

		It("includes filters and no-results markers in empty text output", func() {
			gainOut := runGain(flagFormat, "text")
			plain := engine.StripANSI(gainOut)
			Expect(plain).To(ContainSubstring("0 cmds · 0 → 0 tokens (0.0% saved)"))
			Expect(plain).NotTo(ContainSubstring("filters:"))
			Expect(plain).To(ContainSubstring(noResultsMsg))
		})
	})

	Context("when validating flags", func() {
		It("rejects invalid gain flags", func() {
			Expect(RunGain([]string{flagFormat, "xml"}, path)).To(HaveOccurred())
			Expect(RunGain([]string{flagSince, "nope"}, path)).To(HaveOccurred())
			Expect(RunGain([]string{flagSince, "2d"}, path)).To(Succeed())
			Expect(RunGain([]string{flagSince, "1w"}, path)).To(Succeed())
			Expect(RunGain([]string{flagFormat, "json", flagTable}, path)).To(HaveOccurred())
			Expect(RunGain([]string{flagLimit, "-2"}, path)).To(MatchError(ContainSubstring("invalid --limit -2")))
		})
	})
})

var _ = Describe("RunHistory", func() {
	var (
		tmpDir     string
		path       string
		runHistory func(args ...string) string
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		path = filepath.Join(tmpDir, "gain.db")
		appendGainMetrics(path, []metrics.RunMetric{
			{
				Timestamp:   time.Now().UTC().Add(-2 * time.Hour),
				Tool:        "go",
				Command:     "go test ./...",
				RawBytes:    1200,
				KeptBytes:   400,
				ExitCode:    0,
				Passthrough: false,
			},
			{
				Timestamp:   time.Now().UTC().Add(-1 * time.Hour),
				Tool:        "git",
				Command:     "git push origin main",
				RawBytes:    500,
				KeptBytes:   500,
				ExitCode:    0,
				Passthrough: true,
			},
		})
		runHistory = func(args ...string) string {
			orig := os.Stdout
			r, w, err := os.Pipe()
			Expect(err).NotTo(HaveOccurred())
			os.Stdout = w
			DeferCleanup(func() { os.Stdout = orig })

			Expect(RunHistory(args, path)).To(Succeed())
			Expect(w.Close()).To(Succeed())

			var buf bytes.Buffer
			_, err = io.Copy(&buf, r)
			Expect(err).NotTo(HaveOccurred())
			Expect(r.Close()).To(Succeed())
			return buf.String()
		}
	})

	Context("when rendering history output", func() {
		It("renders history as JSON and CSV", func() {
			jsonOut := runHistory(flagFormat, "json")
			Expect(jsonOut).To(ContainSubstring(`"dataset": "history"`))

			csvOut := runHistory(flagFormat, "csv")
			Expect(csvOut).To(ContainSubstring("dataset,period,since,tool_filter"))
			Expect(csvOut).To(ContainSubstring("history"))
		})

		It("omits the tool column in history text", func() {
			out := runHistory(flagFormat, "text")
			Expect(out).To(ContainSubstring("ccp history"))
			Expect(out).To(ContainSubstring("showing 2 of 2 rows"))
			Expect(out).To(ContainSubstring("TIMESTAMP"))
			Expect(out).To(ContainSubstring("STATUS"))
			Expect(out).To(ContainSubstring("SAVINGS"))
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, "TIMESTAMP") {
					Expect(line).NotTo(ContainSubstring("TOOL"))
					break
				}
			}
		})

		It("includes source attribution in global history text, json, and csv output", func() {
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)

			repoOne := filepath.Join(tmpDir, "repo-one")
			repoTwo := filepath.Join(tmpDir, "repo-two")
			appendGlobalWorkspaceMetrics(home, repoOne, []metrics.RunMetric{
				{Timestamp: time.Now().UTC().Add(-2 * time.Hour), Tool: "go", Command: "go test ./...", RawBytes: 1200, KeptBytes: 400},
			})
			appendGlobalWorkspaceMetrics(home, repoTwo, []metrics.RunMetric{
				{Timestamp: time.Now().UTC().Add(-time.Hour), Tool: "git", Command: "git status", RawBytes: 500, KeptBytes: 500},
			})

			textOut := runHistory(flagGlobal, flagFormat, "text")
			Expect(textOut).To(ContainSubstring("ccp history [global]"))
			Expect(textOut).To(ContainSubstring("SOURCE"))
			Expect(textOut).To(ContainSubstring("repo-one"))
			Expect(textOut).To(ContainSubstring("repo-two"))

			jsonOut := runHistory(flagGlobal, flagFormat, "json")
			Expect(jsonOut).To(ContainSubstring(`"source"`))

			csvOut := runHistory(flagGlobal, flagFormat, "csv")
			Expect(csvOut).To(ContainSubstring("timestamp,source,command"))
		})

		It("includes current repo history when the registry entry exists without metrics", func() {
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)

			repoRoot := filepath.Join(tmpDir, "legacy-repo")
			Expect(os.MkdirAll(repoRoot, 0o755)).To(Succeed())
			repoMetricsPath := filepath.Join(repoRoot, ".ccp", "gain.db")
			appendGainMetrics(repoMetricsPath, []metrics.RunMetric{
				{Timestamp: time.Now().UTC().Add(-time.Hour), Tool: "go", Command: "go test ./...", RawBytes: 1200, KeptBytes: 400},
			})
			Expect(workspaces.UpsertPath(workspaces.PathForHome(home), repoRoot, "")).To(Succeed())

			var out string
			Expect(runInDir(repoRoot, func() error {
				var err error
				out, err = captureStdout(func() error {
					return RunHistory([]string{flagGlobal, flagFormat, "json"}, repoMetricsPath)
				})
				return err
			})).To(Succeed())

			Expect(out).To(ContainSubstring(`"source"`))
			Expect(out).To(ContainSubstring(`"go test ./..."`))
		})

		It("falls back to current repo history when the workspace registry is unreadable", func() {
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)

			registryPath := workspaces.PathForHome(home)
			Expect(os.MkdirAll(filepath.Dir(registryPath), 0o755)).To(Succeed())
			Expect(os.WriteFile(registryPath, []byte("not-a-bolt-db"), 0o644)).To(Succeed())

			repoRoot := filepath.Join(tmpDir, "broken-history-registry-repo")
			Expect(os.MkdirAll(repoRoot, 0o755)).To(Succeed())
			repoMetricsPath := filepath.Join(repoRoot, ".ccp", "gain.db")
			appendGainMetrics(repoMetricsPath, []metrics.RunMetric{
				{Timestamp: time.Now().UTC().Add(-time.Hour), Tool: "go", Command: "go test ./...", RawBytes: 1200, KeptBytes: 400},
			})

			var out string
			Expect(runInDir(repoRoot, func() error {
				var err error
				out, err = captureStdout(func() error {
					return RunHistory([]string{flagGlobal, flagFormat, "json"}, repoMetricsPath)
				})
				return err
			})).To(Succeed())

			Expect(out).To(ContainSubstring(`"source"`))
			Expect(out).To(ContainSubstring(`"go test ./..."`))
		})

		It("warns when skipping corrupt registered workspaces during global history aggregation", func() {
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)

			repo := filepath.Join(tmpDir, "repo")
			appendGlobalWorkspaceMetrics(home, repo, []metrics.RunMetric{
				{Timestamp: time.Now().UTC().Add(-2 * time.Hour), Tool: "go", Command: "go test ./...", RawBytes: 1200, KeptBytes: 400},
			})

			corruptRepo := filepath.Join(tmpDir, "corrupt-history-repo")
			corruptMetricsPath := filepath.Join(corruptRepo, ".ccp", "gain.db")
			Expect(os.MkdirAll(filepath.Dir(corruptMetricsPath), 0o755)).To(Succeed())
			Expect(os.WriteFile(corruptMetricsPath, []byte("not-a-bolt-db"), 0o644)).To(Succeed())
			Expect(workspaces.UpsertPath(workspaces.PathForHome(home), corruptRepo, corruptMetricsPath)).To(Succeed())

			var stdout string
			stderr, err := captureStderrOutput(func() error {
				var runErr error
				stdout, runErr = captureStdout(func() error {
					return RunHistory([]string{flagGlobal, flagFormat, "json"}, path)
				})
				return runErr
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(stdout).To(ContainSubstring(`"dataset": "history"`))
			Expect(stdout).To(ContainSubstring(`"source"`))
			Expect(stdout).To(ContainSubstring(`"go test ./..."`))
			Expect(stderr).To(ContainSubstring("ccp history --global: warning: skipped workspace"))
			Expect(stderr).To(ContainSubstring(corruptRepo))
			Expect(stderr).To(ContainSubstring(corruptMetricsPath))
			Expect(stderr).To(ContainSubstring("results exclude 1 workspace(s)"))
		})

		It("applies the text limit to history output", func() {
			Expect(os.Remove(path)).To(Succeed())
			now := time.Now().UTC()
			for i := range 20 {
				appendGainMetrics(path, []metrics.RunMetric{{
					Timestamp: now.Add(time.Duration(-i) * time.Minute),
					Tool:      "go",
					Command:   fmt.Sprintf("go test ./pkg/%02d", i),
					RawBytes:  1200,
					KeptBytes: 400,
				}})
			}

			out := runHistory(flagFormat, "text")
			Expect(out).To(ContainSubstring("showing 15 of 20 rows, use --limit N to see more"))
			Expect(out).To(ContainSubstring("go test ./pkg/00"))
			Expect(out).NotTo(ContainSubstring("go test ./pkg/19"))
		})

		It("applies the text limit to global history output", func() {
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)

			repo := filepath.Join(tmpDir, "repo")
			now := time.Now().UTC()
			rows := make([]metrics.RunMetric, 0, 20)
			for i := range 20 {
				rows = append(rows, metrics.RunMetric{
					Timestamp: now.Add(time.Duration(-i) * time.Minute),
					Tool:      "go",
					Command:   fmt.Sprintf("go test ./pkg/%02d", i),
					RawBytes:  1200,
					KeptBytes: 400,
				})
			}
			Expect(os.Remove(path)).To(Succeed())
			appendGlobalWorkspaceMetrics(home, repo, rows)

			globalOut := runHistory(flagGlobal, flagLimit, "5")
			Expect(globalOut).To(ContainSubstring("showing 5 of 20 rows, use --limit N to see more"))
			Expect(globalOut).To(ContainSubstring("ccp history [global]"))
			Expect(globalOut).To(ContainSubstring("go test ./pkg/00"))
			Expect(globalOut).NotTo(ContainSubstring("go test ./pkg/19"))

			unlimitedOut := runHistory(flagGlobal, flagLimit, "0")
			Expect(unlimitedOut).To(ContainSubstring("showing 20 of 20 rows"))
			Expect(unlimitedOut).NotTo(ContainSubstring(", use --limit N to see more"))
		})

		It("ignores --limit for non-text history output", func() {
			jsonOut := runHistory(flagFormat, "json", flagLimit, "1")
			var env historyEnvelope
			Expect(json.Unmarshal([]byte(jsonOut), &env)).To(Succeed())
			Expect(env.Rows).To(HaveLen(2))

			csvOut := runHistory(flagFormat, "csv", flagLimit, "1")
			Expect(strings.Count(strings.TrimSpace(csvOut), "\n")).To(Equal(2))
		})
	})

	Context("when applying shared history filters", func() {
		BeforeEach(func() {
			Expect(os.Remove(path)).To(Succeed())
			appendGainMetrics(path, []metrics.RunMetric{
				{
					Timestamp:   time.Now().UTC().Add(-3 * time.Hour),
					Tool:        "go",
					Command:     "go test ./...",
					RawBytes:    1200,
					KeptBytes:   400,
					ExitCode:    0,
					Passthrough: false,
				},
				{
					Timestamp:   time.Now().UTC().Add(-2 * time.Hour),
					Tool:        "git",
					Command:     "git push origin main",
					RawBytes:    500,
					KeptBytes:   500,
					ExitCode:    1,
					Passthrough: true,
				},
				{
					Timestamp:   time.Now().UTC().Add(-1 * time.Hour),
					Tool:        "git",
					Command:     "git pull origin main",
					RawBytes:    450,
					KeptBytes:   450,
					ExitCode:    2,
					Passthrough: true,
				},
			})
		})

		It("applies filters and keeps newest-first order", func() {
			out := runHistory(flagFormat, "json", "--tool", "git", "--failed")

			var env historyEnvelope
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env.Filters.Failed).To(BeTrue())
			Expect(env.Filters.Tool).To(Equal("git"))
			Expect(env.Rows).To(HaveLen(2))
			for _, row := range env.Rows {
				Expect(row.Tool).To(Equal("git"))
				Expect(row.Failed).To(BeTrue())
			}
			Expect(env.Rows[0].Timestamp.Before(env.Rows[1].Timestamp)).To(BeFalse())
		})
	})

	Context("when filtering canonical passthrough tools", func() {
		BeforeEach(func() {
			Expect(os.Remove(path)).To(Succeed())
			appendGainMetrics(path, []metrics.RunMetric{
				{Timestamp: time.Now().UTC().Add(-2 * time.Hour), Tool: "ls", Command: "ls", RawBytes: 300, KeptBytes: 300, ExitCode: 0, Passthrough: true},
				{Timestamp: time.Now().UTC().Add(-1 * time.Hour), Tool: "unknown", Command: "echo a && echo b", RawBytes: 20, KeptBytes: 20, ExitCode: 0, Passthrough: true},
			})
		})

		It("filters recognized passthrough rows by canonical tool", func() {
			out := runHistory(flagFormat, "json", "--tool", "ls")

			var env historyEnvelope
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env.Rows).To(HaveLen(1))
			Expect(env.Rows[0].Tool).To(Equal("ls"))
			Expect(env.Rows[0].Passthrough).To(BeTrue())
		})
	})

	Context("when the history database is empty", func() {
		BeforeEach(func() {
			Expect(os.Remove(path)).To(Succeed())
		})

		It("includes filters and no-results markers in text output", func() {
			out := runHistory(flagFormat, "text")
			Expect(out).To(ContainSubstring("ccp history"))
			Expect(out).NotTo(ContainSubstring("filters:"))
			Expect(out).To(ContainSubstring(noResultsMsg))
		})

		It("rejects gain-only flags", func() {
			Expect(RunHistory([]string{flagPeriod, "day"}, path)).To(MatchError(ContainSubstring("--period is only valid for gain")))
			Expect(RunHistory([]string{flagTable}, path)).To(MatchError(ContainSubstring("--table is only valid for gain")))
			Expect(RunHistory([]string{flagLimit, "-2"}, path)).To(MatchError(ContainSubstring("invalid --limit -2")))
		})
	})
})

var _ = Describe("gain formatting helpers", func() {
	DescribeTable("window and split helpers",
		func(period string, n int, wantDays int, wantDuration time.Duration, wantSplit int) {
			Expect(trendWindowDays(period)).To(Equal(wantDays))
			Expect(durationForPeriod(period)).To(Equal(wantDuration))
			Expect(trendSplitIndex(n, period)).To(Equal(wantSplit))
		},
		Entry("day windows split in half", "day", 6, 1, 24*time.Hour, 3),
		Entry("short week windows do not split", "week", 2, 7, 7*24*time.Hour, 0),
		Entry("three-week windows split at the minimum valid boundary", "week", 3, 7, 7*24*time.Hour, 1),
		Entry("medium week windows split evenly", "week", 4, 7, 7*24*time.Hour, 2),
		Entry("long week windows keep the trailing three buckets together", "week", 8, 7, 7*24*time.Hour, 5),
		Entry("short month windows do not split", "month", 3, 30, 30*24*time.Hour, 0),
		Entry("month windows begin splitting at four buckets", "month", 4, 30, 30*24*time.Hour, 2),
		Entry("month windows up to seven buckets split evenly", "month", 7, 30, 30*24*time.Hour, 3),
		Entry("long month windows keep the trailing seven buckets together", "month", 10, 30, 30*24*time.Hour, 3),
		Entry("unknown periods disable windows and fall back to even splits", "custom", 5, 0, time.Duration(0), 2),
	)

	DescribeTable("effectiveWindowSince bounds summary lookbacks",
		func(base time.Duration, period string, want time.Duration) {
			Expect(effectiveWindowSince(base, period)).To(Equal(want))
		},
		Entry("keeps the original base for unknown periods", 2*time.Hour, "custom", 2*time.Hour),
		Entry("uses the full window when no base is provided", time.Duration(0), "day", 24*time.Hour),
		Entry("uses the full window when the base is negative", -24*time.Hour, "week", 7*24*time.Hour),
		Entry("keeps smaller explicit bases", 2*time.Hour, "day", 2*time.Hour),
		Entry("keeps explicit bases equal to the period window", 7*24*time.Hour, "week", 7*24*time.Hour),
		Entry("caps larger bases at the period window", 14*24*time.Hour, "week", 7*24*time.Hour),
	)

	DescribeTable("display and status helpers",
		func(value string, fallback string, row metrics.HistoryRow, wantFilter string, wantStatus string) {
			Expect(displayFilter(value, fallback)).To(Equal(wantFilter))
			Expect(historyStatus(row)).To(Equal(wantStatus))
		},
		Entry("uses fallbacks for blank filters and passthrough history", "", "*", metrics.HistoryRow{Passthrough: true}, "*", "passthrough"),
		Entry("keeps explicit filters and reports failures", "go", "*", metrics.HistoryRow{Failed: true}, "go", "failed"),
		Entry("reports successful proxied rows as ok", "git", "*", metrics.HistoryRow{}, "git", "ok"),
	)

	It("formats compact filter suffixes and tool summary text", func() {
		Expect(compactFilterSuffix(filtersEnvelope{})).To(BeEmpty())
		Expect(compactFilterSuffix(filtersEnvelope{Since: "2d", Tool: "go", Failed: true}, "", "global")).To(Equal(" [since=2d tool=go failed-only global]"))

		rows := []metrics.SummaryToolRow{
			{Tool: "git", Commands: 2, EstimatedSavingsPct: 12.0, EstimatedSavedTokens: 10},
			{Tool: "go", Commands: 5, EstimatedSavingsPct: 72.0, EstimatedSavedTokens: 120},
			{Tool: "grep", Commands: 4, EstimatedSavingsPct: 45.0, EstimatedSavedTokens: 40},
		}
		Expect(toolsSummaryText(rows)).To(Equal("go (120 / 72%) · grep (40 / 45%) · git (10 / 12%)"))
	})

	DescribeTable("formats compact saved-token values across display ranges",
		func(input int64, want string) {
			Expect(formatCompactSavedTokens(input)).To(Equal(want))
		},
		Entry("leaves small values unscaled", int64(999), "999"),
		Entry("uses one decimal for low thousands", int64(1500), "1.5k"),
		Entry("rounds higher thousands to whole k values", int64(12_345), "12k"),
		Entry("uses one decimal for low millions", int64(1_250_000), "1.2m"),
		Entry("rounds higher millions to whole m values", int64(15_400_000), "15m"),
		Entry("preserves sign for scaled negatives", int64(-1500), "-1.5k"),
	)

	DescribeTable("styles compact gain headlines with threshold-based verdict colors",
		func(pct float64, wantVerdict string) {
			headline := styleCompactGainHeadline(metrics.SummaryTotal{
				Commands:              1,
				EstimatedInputTokens:  400,
				EstimatedOutputTokens: 200,
				EstimatedSavingsPct:   pct,
			}, filtersEnvelope{Tool: "go"}, "global")

			Expect(headline).To(ContainSubstring(wantVerdict))
			Expect(engine.StripANSI(headline)).To(Equal(fmt.Sprintf("1 cmds · 400 → 200 tokens (%s saved) [tool=go global]", formatPercentText(pct))))
		},
		Entry("uses gray below ten percent", 9.9, compactGainColors.verdictGray.Sprint("9.9% saved")),
		Entry("uses amber at ten percent", 10.0, compactGainColors.verdictAmber.Sprint("10.0% saved")),
		Entry("keeps amber below thirty percent", 29.9, compactGainColors.verdictAmber.Sprint("29.9% saved")),
		Entry("uses green at thirty percent", 30.0, compactGainColors.verdictGreen.Sprint("30.0% saved")),
	)

	It("styles compact wins values with bold tool names and gray payloads", func() {
		styled := styleCompactGainWinsValue("go (249k / 18%) · find (41k / 75%)")

		Expect(styled).To(ContainSubstring(compactGainColors.bold.Sprint("go")))
		Expect(styled).To(ContainSubstring(compactGainColors.bold.Sprint("find")))
		Expect(styled).To(ContainSubstring(compactGainColors.gray.Sprint("(")))
		Expect(styled).To(ContainSubstring(compactGainColors.gray.Sprint("249k")))
		Expect(styled).To(ContainSubstring(compactGainColors.gray.Sprint(" / ")))
		Expect(styled).To(ContainSubstring(compactGainColors.gray.Sprint("18%")))
		Expect(engine.StripANSI(styled)).To(Equal("go (249k / 18%) · find (41k / 75%)"))
	})

	It("leaves malformed compact wins values unchanged", func() {
		value := "go 18% · find 75%"
		Expect(styleCompactGainWinsValue(value)).To(Equal(value))
	})

	It("styles compact drag values with bold tool names and gray command counts", func() {
		styled := styleCompactGainDragValue("sed (1,398 cmds) · git (397 cmds)")

		Expect(styled).To(ContainSubstring(compactGainColors.bold.Sprint("sed")))
		Expect(styled).To(ContainSubstring(compactGainColors.bold.Sprint("git")))
		Expect(styled).To(ContainSubstring(compactGainColors.gray.Sprint("(1,398 cmds)")))
		Expect(styled).To(ContainSubstring(compactGainColors.gray.Sprint("(397 cmds)")))
		Expect(engine.StripANSI(styled)).To(Equal("sed (1,398 cmds) · git (397 cmds)"))
	})

	It("styles drag values without command suffixes as bold-only tools", func() {
		styled := styleCompactGainDragValue("sed (1,398 cmds) · misc")

		Expect(styled).To(ContainSubstring(compactGainColors.bold.Sprint("misc")))
		Expect(engine.StripANSI(styled)).To(Equal("sed (1,398 cmds) · misc"))
	})

	DescribeTable("styles compact trend lines by direction",
		func(input string, wantDelta string, wantCompare string) {
			styled := styleCompactGainTrendValue(input)

			Expect(styled).To(ContainSubstring(wantDelta))
			Expect(styled).To(ContainSubstring(wantCompare))
			Expect(engine.StripANSI(styled)).To(Equal(input))
		},
		Entry(
			"upward trends use the up style",
			"↑ +12.4 pts week over week (85.9% → 98.3%) · on a roll",
			compactGainColors.trendUp.Sprint("↑ +12.4 pts"),
			compactGainColors.gray.Sprint("(85.9% → 98.3%)"),
		),
		Entry(
			"downward trends use the down style",
			"↓ -0.3 pts week over week (6.0% → 5.7%) · fading",
			compactGainColors.trendDown.Sprint("↓ -0.3 pts"),
			compactGainColors.gray.Sprint("(6.0% → 5.7%)"),
		),
		Entry(
			"flat trends use the flat style",
			"→ flat week over week (52.0% → 52.0%) · holding high",
			compactGainColors.trendFlat.Sprint("→ flat"),
			compactGainColors.gray.Sprint("(52.0% → 52.0%)"),
		),
	)

	It("keeps insufficient trend data plain", func() {
		Expect(styleCompactGainTrendValue(trendInsufficientData)).To(Equal(trendInsufficientData))
	})

	It("styles trend lines without suffixes by fading the comparison block", func() {
		input := "↑ +1.5 pts week over week (40.0% → 41.5%)"
		styled := styleCompactGainTrendValue(input)

		Expect(styled).To(ContainSubstring(compactGainColors.trendUp.Sprint("↑ +1.5 pts")))
		Expect(styled).To(ContainSubstring(compactGainColors.gray.Sprint("(40.0% → 41.5%)")))
		Expect(engine.StripANSI(styled)).To(Equal(input))
	})

	It("leaves unrecognized trend text unchanged", func() {
		input := "custom trend text"
		Expect(styleCompactGainTrendValue(input)).To(Equal(input))
	})

	It("prints compact gain lines with styled labels and aligned output", func() {
		out, err := captureStdout(func() error {
			printCompactGainLines([]labeledLine{{label: "Wins", value: "go (120 / 72%)"}, {label: "Trend", value: "↑ +1.5 pts week over week (40.0% → 41.5%) · uptick"}})
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(compactGainColors.bold.Sprint("Wins ")))
		Expect(out).To(ContainSubstring(compactGainColors.bold.Sprint("Trend")))
		Expect(engine.StripANSI(out)).To(ContainSubstring("Wins  : go (120 / 72%)"))
		Expect(engine.StripANSI(out)).To(ContainSubstring("Trend : ↑ +1.5 pts week over week (40.0% → 41.5%) · uptick"))
	})

	It("prints nothing for empty compact gain lines", func() {
		out, err := captureStdout(func() error {
			printCompactGainLines(nil)
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeEmpty())
	})

	DescribeTable("formats wins and drag tool summaries deterministically",
		func(rows []metrics.SummaryToolRow, wantWins string, wantDrag string) {
			Expect(topWinsText(rows)).To(Equal(wantWins))
			Expect(dragToolsText(rows)).To(Equal(wantDrag))
		},
		Entry("drops zero-saved tools from wins and keeps only weak tools in drags", []metrics.SummaryToolRow{
			{Tool: "git", Commands: 5, EstimatedSavingsPct: 12.0, EstimatedSavedTokens: 5},
			{Tool: "go", Commands: 4, EstimatedSavingsPct: 72.0, EstimatedSavedTokens: 120},
			{Tool: "grep", Commands: 4, EstimatedSavingsPct: 45.0, EstimatedSavedTokens: 40},
			{Tool: "sed", Commands: 9, EstimatedSavingsPct: 60.0, EstimatedSavedTokens: 0},
		}, "go (120 / 72%) · grep (40 / 45%) · git (5 / 12%)", "git (5 cmds)"),
		Entry("falls back to all tools for drags when none are weak", []metrics.SummaryToolRow{
			{Tool: "go", Commands: 10, EstimatedSavingsPct: 80.0, EstimatedSavedTokens: 100},
			{Tool: "grep", Commands: 12, EstimatedSavingsPct: 40.0, EstimatedSavedTokens: 80},
			{Tool: "git", Commands: 12, EstimatedSavingsPct: 60.0, EstimatedSavedTokens: 70},
			{Tool: "awk", Commands: 2, EstimatedSavingsPct: 50.0, EstimatedSavedTokens: 10},
		}, "go (100 / 80%) · grep (80 / 40%) · git (70 / 60%)", "grep (12 cmds) · git (12 cmds) · go (10 cmds)"),
	)

	DescribeTable("summarizes insight lines with the correct fallback",
		func(rows []metrics.SummaryToolRow, expected []labeledLine) {
			Expect(summaryInsightLines(rows)).To(Equal(expected))
		},
		Entry("returns nil for empty rows", nil, nil),
		Entry("returns wins and drags when the split is meaningful and both sides render", []metrics.SummaryToolRow{
			{Tool: "go", Commands: 10, EstimatedSavingsPct: 78.0, EstimatedSavedTokens: 100},
			{Tool: "git", Commands: 8, EstimatedSavingsPct: 4.0, EstimatedSavedTokens: 5},
		}, []labeledLine{
			{label: "Wins", value: "go (100 / 78%) · git (5 / 4%)"},
			{label: "Drag", value: "git (8 cmds)"},
		}),
		Entry("falls back to the tools line when wins would be empty", []metrics.SummaryToolRow{
			{Tool: "go", Commands: 10, EstimatedSavingsPct: 78.0, EstimatedSavedTokens: 0},
			{Tool: "git", Commands: 8, EstimatedSavingsPct: 4.0, EstimatedSavedTokens: 0},
		}, []labeledLine{
			{label: "Tools", value: "go (0 / 78%) · git (0 / 4%)"},
		}),
	)

	DescribeTable("splitting wins and drags only for meaningful contrast",
		func(rows []metrics.SummaryToolRow, want bool) {
			Expect(shouldSplitWinsDrags(rows)).To(Equal(want))
		},
		Entry("single tool", []metrics.SummaryToolRow{
			{Tool: "go", Commands: 10, EstimatedSavingsPct: 42, EstimatedSavedTokens: 100},
		}, false),
		Entry("two close tools", []metrics.SummaryToolRow{
			{Tool: "go", Commands: 10, EstimatedSavingsPct: 42, EstimatedSavedTokens: 100},
			{Tool: "git", Commands: 8, EstimatedSavingsPct: 38, EstimatedSavedTokens: 80},
		}, false),
		Entry("two far-apart tools", []metrics.SummaryToolRow{
			{Tool: "go", Commands: 10, EstimatedSavingsPct: 78, EstimatedSavedTokens: 100},
			{Tool: "git", Commands: 8, EstimatedSavingsPct: 4, EstimatedSavedTokens: 5},
		}, true),
		Entry("three mixed tools", []metrics.SummaryToolRow{
			{Tool: "go", Commands: 10, EstimatedSavingsPct: 81, EstimatedSavedTokens: 100},
			{Tool: "grep", Commands: 8, EstimatedSavingsPct: 63, EstimatedSavedTokens: 80},
			{Tool: "git", Commands: 7, EstimatedSavingsPct: 12, EstimatedSavedTokens: 10},
		}, true),
	)

	DescribeTable("split boundaries stay exact at strong, weak, and spread thresholds",
		func(rows []metrics.SummaryToolRow, want bool) {
			Expect(shouldSplitWinsDrags(rows)).To(Equal(want))
		},
		Entry("two rows split exactly at the 35/20/20 thresholds", []metrics.SummaryToolRow{
			{Tool: "go", Commands: 10, EstimatedSavingsPct: 35, EstimatedSavedTokens: 100},
			{Tool: "git", Commands: 8, EstimatedSavingsPct: 15, EstimatedSavedTokens: 5},
		}, true),
		Entry("two rows do not split when the spread is just below twenty", []metrics.SummaryToolRow{
			{Tool: "go", Commands: 10, EstimatedSavingsPct: 35, EstimatedSavedTokens: 100},
			{Tool: "git", Commands: 8, EstimatedSavingsPct: 16, EstimatedSavedTokens: 5},
		}, false),
		Entry("three rows split exactly at the ten-point spread threshold", []metrics.SummaryToolRow{
			{Tool: "go", Commands: 10, EstimatedSavingsPct: 35, EstimatedSavedTokens: 100},
			{Tool: "grep", Commands: 9, EstimatedSavingsPct: 25, EstimatedSavedTokens: 50},
			{Tool: "git", Commands: 8, EstimatedSavingsPct: 20, EstimatedSavedTokens: 5},
		}, true),
	)

	It("treats twenty-percent tools as drags and keeps top wins deterministic on exact ties", func() {
		rows := []metrics.SummaryToolRow{
			{Tool: "grep", Commands: 5, EstimatedSavingsPct: 60, EstimatedSavedTokens: 40},
			{Tool: "go", Commands: 5, EstimatedSavingsPct: 70, EstimatedSavedTokens: 40},
			{Tool: "git", Commands: 8, EstimatedSavingsPct: 20, EstimatedSavedTokens: 5},
		}

		Expect(topWinsText(rows)).To(Equal("go (40 / 70%) · grep (40 / 60%) · git (5 / 20%)"))
		Expect(dragToolsText(rows)).To(Equal("git (8 cmds)"))
	})

	It("formats trend summaries with one decimal precision", func() {
		rows := []metrics.PeriodRow{
			{BucketStart: "2026-03-01", EstimatedSavingsPct: 31.4},
			{BucketStart: "2026-03-02", EstimatedSavingsPct: 31.4},
			{BucketStart: "2026-03-03", EstimatedSavingsPct: 37.6},
			{BucketStart: "2026-03-04", EstimatedSavingsPct: 37.6},
		}
		Expect(trendSummaryText(rows, "week")).To(Equal("↑ +6.2 pts week over week (31.4% → 37.6%) · clear gain"))
	})

	It("uses richer deterministic suffixes for flat and downward trends", func() {
		flatRows := []metrics.PeriodRow{
			{BucketStart: "2026-03-01", EstimatedSavingsPct: 52.0},
			{BucketStart: "2026-03-02", EstimatedSavingsPct: 52.0},
			{BucketStart: "2026-03-03", EstimatedSavingsPct: 52.0},
			{BucketStart: "2026-03-04", EstimatedSavingsPct: 52.0},
		}
		downRows := []metrics.PeriodRow{
			{BucketStart: "2026-03-01", EstimatedSavingsPct: 13.0},
			{BucketStart: "2026-03-02", EstimatedSavingsPct: 13.0},
			{BucketStart: "2026-03-03", EstimatedSavingsPct: 10.9},
			{BucketStart: "2026-03-04", EstimatedSavingsPct: 10.9},
		}

		Expect(trendSummaryText(flatRows, "week")).To(Equal("→ flat week over week (52.0% → 52.0%) · holding high"))
		Expect(trendSummaryText(downRows, "week")).To(Equal("↓ -2.1 pts week over week (13.0% → 10.9%) · slipping"))
	})

	DescribeTable("trend labels and buckets",
		func(period string, pct float64, diff float64, wantLabel string, wantSavings string, wantDelta string) {
			Expect(trendLabel(period)).To(Equal(wantLabel))
			Expect(trendSavingsBucket(pct)).To(Equal(wantSavings))
			Expect(trendDeltaBucket(diff)).To(Equal(wantDelta))
		},
		Entry("day trends use day labels and no-savings buckets", "day", 0.0, 1.5, "day over day", "none", "small"),
		Entry("month trends use month labels and medium deltas", "month", 35.0, 3.0, "month over month", "mid", "medium"),
		Entry("other trends default to weeks and large deltas", "week", 75.0, 8.0, "week over week", "extreme", "large"),
	)

	DescribeTable("trend bucket boundaries stay stable",
		func(pct float64, diff float64, wantSavings string, wantDelta string) {
			Expect(trendSavingsBucket(pct)).To(Equal(wantSavings))
			Expect(trendDeltaBucket(diff)).To(Equal(wantDelta))
		},
		Entry("zero savings stays in none and sub-two deltas are small", 0.0, 1.99, "none", "small"),
		Entry("twenty percent starts mid and two-point deltas are medium", 20.0, 2.0, "mid", "medium"),
		Entry("forty percent starts high and negative medium deltas stay medium", 40.0, -5.99, "high", "medium"),
		Entry("sixty percent starts extreme and six-point deltas are large", 60.0, 6.0, "extreme", "large"),
	)

	DescribeTable("trend savings buckets cover every interval edge",
		func(pct float64, want string) {
			Expect(trendSavingsBucket(pct)).To(Equal(want))
		},
		Entry("negative savings stay none", -0.1, "none"),
		Entry("exact zero stays none", 0.0, "none"),
		Entry("positive savings below twenty are low", 19.9, "low"),
		Entry("exactly twenty starts mid", 20.0, "mid"),
		Entry("below forty stays mid", 39.9, "mid"),
		Entry("exactly forty starts high", 40.0, "high"),
		Entry("below sixty stays high", 59.9, "high"),
		Entry("exactly sixty starts extreme", 60.0, "extreme"),
	)

	DescribeTable("trend delta buckets cover every interval edge",
		func(diff float64, want string) {
			Expect(trendDeltaBucket(diff)).To(Equal(want))
		},
		Entry("zero diff is small", 0.0, "small"),
		Entry("just below two is small", 1.99, "small"),
		Entry("exactly two is medium", 2.0, "medium"),
		Entry("just below six is medium", 5.99, "medium"),
		Entry("exactly six is large", 6.0, "large"),
	)

	DescribeTable("deterministic variant selection stays stable",
		func(variants []string, earlier float64, recent float64, want string) {
			Expect(deterministicVariant(variants, earlier, recent)).To(Equal(want))
		},
		Entry("empty variant sets return an empty string", nil, 10.0, 12.0, ""),
		Entry("positive seeds choose a stable index", []string{"first", "second"}, 10.0, 10.1, "second"),
		Entry("negative seeds wrap back into the variant list", []string{"first", "second"}, -10.0, -10.1, "second"),
	)

	DescribeTable("trend suffix helpers pick the expected vocabulary branches",
		func(deltaBucket string, savingsBucket string, earlier float64, recent float64, wantUp string, wantDown string, wantFlat string) {
			if wantUp != "" {
				Expect(trendSuffixUp(deltaBucket, savingsBucket, earlier, recent)).To(Equal(wantUp))
			}
			if wantDown != "" {
				Expect(trendSuffixDown(deltaBucket, savingsBucket, earlier, recent)).To(Equal(wantDown))
			}
			if wantFlat != "" {
				Expect(trendSuffixFlat(savingsBucket, earlier, recent)).To(Equal(wantFlat))
			}
		},
		Entry("medium gains in high savings use the high-gain vocabulary", "medium", "high", 50.0, 54.0, "gaining", "", ""),
		Entry("large gains outside high savings use the general gain vocabulary", "large", "mid", 30.0, 37.0, "clear gain", "", ""),
		Entry("large declines in low savings use the weak-decline vocabulary", "large", "low", 10.0, 3.0, "", "backslide", ""),
		Entry("medium declines in high savings use the higher-savings decline vocabulary", "medium", "high", 50.0, 46.0, "", "losing ground", ""),
		Entry("mid flat savings use the neutral flat vocabulary", "", "mid", 30.0, 30.0, "", "", "holding"),
	)

	DescribeTable("trend suffix branches are deterministic across buckets",
		func(earlier float64, recent float64, want string) {
			Expect(trendSuffix(earlier, recent)).To(Equal(want))
		},
		Entry("small upward moves use the small-up vocabulary", 40.0, 41.0, "uptick"),
		Entry("medium upward moves in mid savings use the mid-up vocabulary", 30.0, 34.0, "gaining"),
		Entry("large upward moves in extreme savings use the large-up vocabulary", 55.0, 62.0, "clear gain"),
		Entry("small downward moves in low savings use the weak-down vocabulary", 10.0, 9.0, "thin"),
		Entry("medium downward moves in higher savings use the high-down vocabulary", 50.0, 46.0, "losing ground"),
		Entry("large downward moves in higher savings use the large-down vocabulary", 70.0, 60.0, "hard fade"),
		Entry("flat low savings use the flat-low vocabulary", 10.0, 10.0, "stuck low"),
		Entry("flat high savings use the flat-high vocabulary", 70.0, 70.0, "holding high"),
		Entry("flat mid savings use the neutral flat vocabulary", 30.0, 30.0, "holding"),
	)

	DescribeTable("trend summaries handle ordering and boundary diffs",
		func(rows []metrics.PeriodRow, period string, want string) {
			Expect(trendSummaryText(rows, period)).To(Equal(want))
		},
		Entry("reports insufficient data with one row", []metrics.PeriodRow{
			{BucketStart: "2026-03-01", EstimatedSavingsPct: 31.4},
		}, "week", "insufficient data"),
		Entry("reports insufficient data when the split index collapses", []metrics.PeriodRow{
			{BucketStart: "2026-03-01", EstimatedSavingsPct: 31.4},
			{BucketStart: "2026-03-02", EstimatedSavingsPct: 31.4},
			{BucketStart: "2026-03-03", EstimatedSavingsPct: 31.4},
		}, "month", "insufficient data"),
		Entry("sorts rows before summarizing two-row trends", []metrics.PeriodRow{
			{BucketStart: "2026-03-02", EstimatedSavingsPct: 55.0},
			{BucketStart: "2026-03-01", EstimatedSavingsPct: 45.0},
		}, "day", "↑ +10.0 pts day over day (45.0% → 55.0%) · clear gain"),
		Entry("treats a +0.05 diff as flat", []metrics.PeriodRow{
			{BucketStart: "2026-03-01", EstimatedSavingsPct: 52.00},
			{BucketStart: "2026-03-02", EstimatedSavingsPct: 52.05},
		}, "week", "→ flat week over week (52.0% → 52.0%) · dialed in"),
		Entry("treats a -0.05 diff as flat", []metrics.PeriodRow{
			{BucketStart: "2026-03-01", EstimatedSavingsPct: 52.05},
			{BucketStart: "2026-03-02", EstimatedSavingsPct: 52.00},
		}, "week", "→ flat week over week (52.0% → 52.0%) · dialed in"),
	)

	DescribeTable("formatTrendSummary keeps direct branch thresholds stable",
		func(earlier float64, recent float64, period string, want string) {
			Expect(formatTrendSummary(earlier, recent, period)).To(Equal(want))
		},
		Entry("uses the upward branch just above the threshold", 10.0, 10.06, "day", "↑ +0.1 pts day over day (10.0% → 10.1%) · firming"),
		Entry("uses the downward branch just below the threshold", 10.06, 10.0, "day", "↓ -0.1 pts day over day (10.1% → 10.0%) · fading"),
		Entry("keeps the flat branch at the exact positive threshold", 10.0, 10.05, "day", "→ flat day over day (10.0% → 10.1%) · flatline"),
		Entry("keeps the flat branch at the exact negative threshold", 10.05, 10.0, "day", "→ flat day over day (10.1% → 10.0%) · flatline"),
	)

	DescribeTable("resolves text limits and row limiting consistently",
		func(rows []int, limit int, expected []int, total int) {
			limited, gotTotal := limitRows(rows, limit)
			Expect(limited).To(Equal(expected))
			Expect(gotTotal).To(Equal(total))
		},
		Entry("keeps all rows when limit is zero", []int{1, 2, 3}, 0, []int{1, 2, 3}, 3),
		Entry("uses the default limit when a negative limit is provided", slices.Collect(func(yield func(int) bool) {
			for i := range 16 {
				if !yield(i) {
					return
				}
			}
		}), -1, slices.Collect(func(yield func(int) bool) {
			for i := range defaultTextLimit {
				if !yield(i) {
					return
				}
			}
		}), 16),
		Entry("keeps all rows when the resolved limit exactly matches the row count", []int{1, 2, 3}, 3, []int{1, 2, 3}, 3),
		Entry("keeps all rows when the resolved limit exceeds the row count", []int{1, 2, 3}, 5, []int{1, 2, 3}, 3),
	)

	DescribeTable("formats table summary lines",
		func(displayed int, total int, noun string, want string) {
			Expect(tableSummaryLine(displayed, total, noun)).To(Equal(want))
		},
		Entry("reports full tables without extra guidance", 3, 3, "rows", "showing 3 of 3 rows"),
		Entry("reports truncated tables with follow-up guidance", 5, 20, "tools", "showing 5 of 20 tools, use --limit N to see more"),
	)

	It("sorts gain table rows by savings first and zero-savings rows by count", func() {
		rows := []metrics.SummaryToolRow{
			{Tool: "git", Commands: 10, EstimatedSavedTokens: 0},
			{Tool: "go", Commands: 8, EstimatedSavedTokens: 20},
			{Tool: "grep", Commands: 12, EstimatedSavedTokens: 20},
			{Tool: "awk", Commands: 9, EstimatedSavedTokens: 0},
		}

		Expect(sortGainTableRows(rows)).To(Equal([]metrics.SummaryToolRow{
			{Tool: "grep", Commands: 12, EstimatedSavedTokens: 20},
			{Tool: "go", Commands: 8, EstimatedSavedTokens: 20},
			{Tool: "git", Commands: 10, EstimatedSavedTokens: 0},
			{Tool: "awk", Commands: 9, EstimatedSavedTokens: 0},
		}))
	})

	It("pads cells and renders text tables with stable alignment", func() {
		Expect(padTableCell("42", 4, true)).To(Equal("   42 "))
		Expect(padTableCell("go", 4, false)).To(Equal(" go   "))
		Expect(padTableCell("long-value", 4, false)).To(Equal(" long-value "))

		table := renderTextTable([]textTableColumn{
			{header: "TOOL"},
			{header: "COUNT", right: true},
		}, [][]string{{"go", "42"}})

		Expect(table).To(Equal(strings.Join([]string{
			"+------+-------+",
			"| TOOL | COUNT |",
			"+------+-------+",
			"| go   |    42 |",
			"+------+-------+",
			"",
		}, "\n")))
	})

	It("renders single-column text tables with stable borders", func() {
		table := renderTextTable([]textTableColumn{
			{header: "VALUE"},
		}, [][]string{{"alpha"}, {"beta"}})

		Expect(table).To(Equal(strings.Join([]string{
			"+-------+",
			"| VALUE |",
			"+-------+",
			"| alpha |",
			"| beta  |",
			"+-------+",
			"",
		}, "\n")))
	})

	It("prints labeled lines with aligned labels and no output for empty lists", func() {
		out, err := captureStdout(func() error {
			printLabeledLines(nil)
			printLabeledLines([]labeledLine{
				{label: "W", value: "one"},
				{label: "Wide", value: "two"},
			})
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("W    : one\nWide : two\n"))
	})

	DescribeTable("renders summary and history tables for empty and populated datasets",
		func(run func() error, expected []string) {
			out, err := captureStdout(run)
			Expect(err).NotTo(HaveOccurred())
			plain := engine.StripANSI(out)
			for _, fragment := range expected {
				Expect(plain).To(ContainSubstring(fragment))
			}
		},
		Entry("prints the compact summary no-results branch", func() error {
			return printCompactGainSummary(filtersEnvelope{Tool: "go"}, metrics.SummaryTotal{}, nil, nil, "week", "", false)
		}, []string{
			"0 cmds · 0 → 0 tokens (0.0% saved) [tool=go]",
			noResultsMsg,
		}),
		Entry("prints the summary table no-results branch with global period tags", func() error {
			return printSummaryTableText(filtersEnvelope{}, metrics.SummaryTotal{Commands: 1}, nil, 5, "week", true)
		}, []string{
			"1 cmds · 0 → 0 tokens (0.0% saved) [period=week global]",
			noResultsMsg,
		}),
		Entry("prints the history table no-results branch", func() error {
			return printHistoryTable(nil, filtersEnvelope{Failed: true}, 5)
		}, []string{
			"ccp history [failed-only]",
			noResultsMsg,
		}),
		Entry("prints the global history table no-results branch", func() error {
			return printGlobalHistoryTable(nil, filtersEnvelope{Tool: "go"}, 5)
		}, []string{
			"ccp history [tool=go global]",
			noResultsMsg,
		}),
	)

	It("prints populated summary and history tables with truncation notices", func() {
		out, err := captureStdout(func() error {
			err := printSummaryTableText(filtersEnvelope{}, metrics.SummaryTotal{
				Commands:              2,
				EstimatedInputTokens:  120,
				EstimatedOutputTokens: 60,
				EstimatedSavingsPct:   50,
			}, []metrics.SummaryToolRow{
				{Tool: "go", Commands: 2, EstimatedInputTokens: 120, EstimatedOutputTokens: 60, EstimatedSavedTokens: 60, EstimatedSavingsPct: 50},
				{Tool: "git", Commands: 1, EstimatedInputTokens: 20, EstimatedOutputTokens: 20, EstimatedSavedTokens: 0, EstimatedSavingsPct: 0},
			}, 1, "", false)
			if err != nil {
				return err
			}
			return printGlobalHistoryTable([]globalHistoryRow{
				{
					HistoryRow: metrics.HistoryRow{
						Timestamp:             time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC),
						Command:               "go test ./really/long/package/name",
						EstimatedSavingsPct:   50,
						EstimatedInputTokens:  10,
						EstimatedOutputTokens: 5,
						EstimatedSavedTokens:  5,
					},
					Source: "/very/long/source/path/for/project",
				},
				{
					HistoryRow: metrics.HistoryRow{
						Timestamp:           time.Date(2026, 3, 26, 11, 0, 0, 0, time.UTC),
						Command:             "git status",
						EstimatedSavingsPct: 0,
					},
					Source: "/short",
				},
			}, filtersEnvelope{}, 1)
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("showing 1 of 2 tools, use --limit N to see more"))
		Expect(out).To(ContainSubstring("| TOOL | COUNT | NATIVE | PROXIED | SAVED | SAVINGS |"))
		Expect(out).To(ContainSubstring("showing 1 of 2 rows, use --limit N to see more"))
		Expect(out).To(ContainSubstring("| TIMESTAMP"))
		Expect(out).To(ContainSubstring("...g/source/path/for/project"))
	})

	DescribeTable("truncateForDisplay branches",
		func(input string, max int, want string) {
			Expect(truncateForDisplay(input, max)).To(Equal(want))
		},
		Entry("negative max behaves like zero", "abcdef", -1, ""),
		Entry("max<=0", "abcdef", 0, ""),
		Entry("returns the original string when max matches the rune length", "abcdef", 6, "abcdef"),
		Entry("max<=3", "abcdef", 3, "abc"),
		Entry("ellipsis", "abcdef", 5, "ab..."),
	)

	DescribeTable("summary query options preserve or collapse periods correctly",
		func(flags reportFlags, opts metrics.QueryOptions, want metrics.QueryOptions) {
			Expect(summaryQueryOptions(flags, opts)).To(Equal(want))
		},
		Entry("collapses the period for compact text summaries", reportFlags{format: "text"}, metrics.QueryOptions{
			Period: "week",
			Since:  14 * 24 * time.Hour,
			Tool:   "go",
			Failed: true,
		}, metrics.QueryOptions{
			Since:  7 * 24 * time.Hour,
			Tool:   "go",
			Failed: true,
		}),
		Entry("keeps the period for text tables", reportFlags{format: "text", table: true}, metrics.QueryOptions{
			Period: "week",
			Since:  14 * 24 * time.Hour,
		}, metrics.QueryOptions{
			Period: "week",
			Since:  14 * 24 * time.Hour,
		}),
		Entry("keeps the period for json output", reportFlags{format: "json"}, metrics.QueryOptions{
			Period: "month",
			Since:  48 * time.Hour,
		}, metrics.QueryOptions{
			Period: "month",
			Since:  48 * time.Hour,
		}),
	)

	DescribeTable("trend day query options prefer the narrower lookback",
		func(opts metrics.QueryOptions, period string, want metrics.QueryOptions) {
			now := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
			Expect(trendDayQueryOptions(opts, period, now)).To(Equal(want))
		},
		Entry("returns a day query without changing unknown windows", metrics.QueryOptions{Since: 2 * time.Hour, Tool: "go"}, "custom", metrics.QueryOptions{
			Period: "day",
			Since:  2 * time.Hour,
			Tool:   "go",
		}),
		Entry("uses the computed lookback when no explicit since is provided", metrics.QueryOptions{Tool: "go"}, "week", metrics.QueryOptions{
			Period: "day",
			Since:  13 * 24 * time.Hour,
			Tool:   "go",
		}),
		Entry("uses the computed lookback when the explicit since exactly matches it", metrics.QueryOptions{Since: 13 * 24 * time.Hour}, "week", metrics.QueryOptions{
			Period: "day",
			Since:  13 * 24 * time.Hour,
		}),
		Entry("keeps a smaller explicit since value", metrics.QueryOptions{Since: 24 * time.Hour, Tool: "go"}, "week", metrics.QueryOptions{
			Period: "day",
			Since:  24 * time.Hour,
			Tool:   "go",
		}),
		Entry("uses the full lookback when the explicit since is larger", metrics.QueryOptions{Since: 30 * 24 * time.Hour}, "week", metrics.QueryOptions{
			Period: "day",
			Since:  13 * 24 * time.Hour,
		}),
	)

	It("aggregates trend rows into earlier and recent windows while ignoring invalid buckets", func() {
		now := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
		rows := aggregateTrendRows([]metrics.PeriodRow{
			{BucketStart: "2026-03-07", Commands: 2, RawBytes: 8, KeptBytes: 4},
			{BucketStart: "2026-03-16", Commands: 1, RawBytes: 4, KeptBytes: 4},
			{BucketStart: "invalid-date", Commands: 9, RawBytes: 100, KeptBytes: 0},
			{BucketStart: "2026-02-20", Commands: 3, RawBytes: 90, KeptBytes: 30},
		}, "week", now)

		Expect(rows).To(Equal([]metrics.PeriodRow{
			{
				Bucket:                "2026-03-07",
				BucketStart:           "2026-03-07",
				BucketEnd:             "2026-03-13",
				Commands:              2,
				RawBytes:              8,
				KeptBytes:             4,
				DroppedBytes:          4,
				DropRatio:             0.5,
				EstimatedInputTokens:  2,
				EstimatedOutputTokens: 1,
				EstimatedSavedTokens:  1,
				EstimatedSavingsPct:   50,
			},
			{
				Bucket:                "2026-03-14",
				BucketStart:           "2026-03-14",
				BucketEnd:             "2026-03-20",
				Commands:              1,
				RawBytes:              4,
				KeptBytes:             4,
				DroppedBytes:          0,
				DropRatio:             0,
				EstimatedInputTokens:  1,
				EstimatedOutputTokens: 1,
				EstimatedSavedTokens:  0,
				EstimatedSavingsPct:   0,
			},
		}))
		Expect(aggregateTrendRows(nil, "custom", now)).To(BeNil())
	})

	It("omits empty trend windows when only one side has data", func() {
		now := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)

		earlierOnly := aggregateTrendRows([]metrics.PeriodRow{
			{BucketStart: "2026-03-07", Commands: 2, RawBytes: 8, KeptBytes: 4},
		}, "week", now)
		Expect(earlierOnly).To(HaveLen(1))
		Expect(earlierOnly[0].BucketStart).To(Equal("2026-03-07"))

		recentOnly := aggregateTrendRows([]metrics.PeriodRow{
			{BucketStart: "2026-03-20", Commands: 1, RawBytes: 4, KeptBytes: 0},
		}, "week", now)
		Expect(recentOnly).To(HaveLen(1))
		Expect(recentOnly[0].BucketStart).To(Equal("2026-03-14"))
	})

	DescribeTable("parses --since values from durations and day or week shorthands",
		func(raw string, want time.Duration, wantErr string) {
			got, err := parseSince(raw)
			if wantErr != "" {
				Expect(err).To(MatchError(wantErr))
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(want))
		},
		Entry("accepts empty values", "", time.Duration(0), ""),
		Entry("accepts native durations", "15m", 15*time.Minute, ""),
		Entry("accepts zero day shorthands", "0d", time.Duration(0), ""),
		Entry("accepts day shorthands", "2d", 48*time.Hour, ""),
		Entry("accepts week shorthands", "3w", 21*24*time.Hour, ""),
		Entry("rejects missing shorthand values", "d", time.Duration(0), "invalid --since \"d\""),
		Entry("rejects negative shorthands", "-1d", time.Duration(0), "invalid --since \"-1d\""),
		Entry("rejects unknown units", "4x", time.Duration(0), "invalid --since \"4x\""),
	)

	DescribeTable("formats integers with stable thousands separators",
		func(value int64, want string) {
			Expect(formatInt(value)).To(Equal(want))
		},
		Entry("keeps short positive numbers unchanged", int64(42), "42"),
		Entry("keeps three-digit positive numbers unchanged", int64(999), "999"),
		Entry("keeps short negative numbers unchanged", int64(-42), "-42"),
		Entry("keeps three-digit negative numbers unchanged", int64(-999), "-999"),
		Entry("formats positive thousands", int64(1234), "1,234"),
		Entry("formats negative thousands", int64(-1234), "-1,234"),
		Entry("formats larger values with repeated grouping", int64(1234567890), "1,234,567,890"),
	)

	Context("global metrics helpers", func() {
		DescribeTable("normalizing global paths",
			func(input string, expected string) {
				Expect(normalizeGlobalPath(input)).To(Equal(expected))
			},
			Entry("trims empty whitespace to an empty path", " \t ", ""),
			Entry("cleans relative paths to absolute paths", "./testdata/../filters", filepath.Clean(func() string {
				abs, err := filepath.Abs("./filters")
				Expect(err).NotTo(HaveOccurred())
				return abs
			}())),
		)

		DescribeTable("selecting warning source labels",
			func(failure globalQueryFailure, expected string) {
				Expect(globalQuerySourceLabel(failure)).To(Equal(expected))
			},
			Entry("prefers the workspace cwd", globalQueryFailure{CWD: "/repo", MetricsPath: "/repo/.ccp/gain.db"}, "/repo"),
			Entry("falls back to the metrics path", globalQueryFailure{MetricsPath: "/repo/.ccp/gain.db"}, "/repo/.ccp/gain.db"),
			Entry("uses an explicit unknown placeholder when no source exists", globalQueryFailure{}, "<unknown>"),
		)

		It("records failures once and falls back to cwd keys when the metrics path is empty", func() {
			session := &globalQuerySession{failures: map[string]globalQueryFailure{}}

			session.recordFailure(globalMetricsSource{CWD: "/repo-a"}, errors.New("first"))
			session.recordFailure(globalMetricsSource{CWD: "/repo-a"}, errors.New("second"))
			session.recordFailure(globalMetricsSource{CWD: "/repo-b", MetricsPath: "/repo-b/.ccp/gain.db"}, nil)

			Expect(session.failures).To(HaveLen(1))
			Expect(session.failures).To(HaveKey("/repo-a"))
			Expect(session.failures["/repo-a"].Err).To(MatchError("first"))
		})

		It("discovers global sources from the registry and current workspace without duplicates", func() {
			baseDir := GinkgoT().TempDir()
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)

			registeredRepo := filepath.Join(baseDir, "registered-repo")
			duplicateRepo := filepath.Join(baseDir, "duplicate-repo")
			missingRepo := filepath.Join(baseDir, "missing-repo")
			currentRepo := filepath.Join(baseDir, "current-repo")
			registeredPath := filepath.Join(registeredRepo, ".ccp", "gain.db")
			missingPath := filepath.Join(missingRepo, ".ccp", "gain.db")
			currentPath := filepath.Join(currentRepo, ".ccp", "gain.db")

			appendGainMetrics(registeredPath, []metrics.RunMetric{
				{Timestamp: time.Date(2026, 3, 26, 8, 0, 0, 0, time.UTC), Tool: "go", Command: "go test ./...", RawBytes: 8, KeptBytes: 4},
			})
			appendGainMetrics(currentPath, []metrics.RunMetric{
				{Timestamp: time.Date(2026, 3, 26, 8, 1, 0, 0, time.UTC), Tool: "git", Command: "git status", RawBytes: 4, KeptBytes: 4},
			})

			registryPath := workspaces.PathForHome(home)
			Expect(workspaces.UpsertPath(registryPath, registeredRepo, registeredPath)).To(Succeed())
			Expect(workspaces.UpsertPath(registryPath, duplicateRepo, registeredPath)).To(Succeed())
			Expect(workspaces.UpsertPath(registryPath, missingRepo, missingPath)).To(Succeed())

			var sources []globalMetricsSource
			Expect(runInDir(currentRepo, func() error {
				var err error
				sources, err = globalMetricsSources(filepath.Join(currentRepo, ".ccp", ".", "gain.db"))
				return err
			})).To(Succeed())

			normalizedSources := make([]globalMetricsSource, 0, len(sources))
			for _, source := range sources {
				normalizedSources = append(normalizedSources, globalMetricsSource{
					CWD:         resolvedPath(source.CWD),
					MetricsPath: resolvedPath(source.MetricsPath),
				})
			}

			Expect(normalizedSources).To(ConsistOf(
				globalMetricsSource{
					CWD:         resolvedPath(duplicateRepo),
					MetricsPath: resolvedPath(registeredPath),
				},
				globalMetricsSource{
					CWD:         resolvedPath(currentRepo),
					MetricsPath: resolvedPath(currentPath),
				},
			))

			entries, err := workspaces.ListPath(registryPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(ContainElement(WithTransform(func(entry workspaces.Workspace) string {
				return resolvedPath(entry.CWD)
			}, Equal(resolvedPath(currentRepo)))))
		})

		DescribeTable("builds the current global metrics source only when a metrics path is available",
			func(currentPath string, expectNil bool) {
				repo := GinkgoT().TempDir()
				var source *globalMetricsSource
				var expectedCWD string
				var expectedMetricsPath string
				Expect(runInDir(repo, func() error {
					expectedCWD = normalizeGlobalPath(".")
					expectedMetricsPath = normalizeGlobalPath(currentPath)
					source = currentGlobalMetricsSource(currentPath)
					return nil
				})).To(Succeed())

				if expectNil {
					Expect(source).To(BeNil())
					return
				}

				Expect(source).NotTo(BeNil())
				Expect(source.CWD).To(Equal(expectedCWD))
				Expect(source.MetricsPath).To(Equal(expectedMetricsPath))
			},
			Entry("returns nil for an empty metrics path", " \t ", true),
			Entry("normalizes a relative metrics path using the current working directory", filepath.Join(".", ".ccp", "gain.db"), false),
		)

		It("writes global warnings in cwd-then-metrics-path order", func() {
			session := &globalQuerySession{
				failures: map[string]globalQueryFailure{
					"b": {CWD: "/repo-b", MetricsPath: "/repo-b/.ccp/gain.db", Err: errors.New("broken b")},
					"z": {CWD: "/repo-a", MetricsPath: "/repo-a/.ccp/z.db", Err: errors.New("broken z")},
					"a": {CWD: "/repo-a", MetricsPath: "/repo-a/.ccp/a.db", Err: errors.New("broken a")},
				},
			}

			stderr, err := captureStderrOutput(func() error {
				session.writeWarnings("gain")
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			first := strings.Index(stderr, "/repo-a (/repo-a/.ccp/a.db): broken a")
			second := strings.Index(stderr, "/repo-a (/repo-a/.ccp/z.db): broken z")
			third := strings.Index(stderr, "/repo-b (/repo-b/.ccp/gain.db): broken b")
			Expect(first).To(BeNumerically(">=", 0))
			Expect(second).To(BeNumerically(">", first))
			Expect(third).To(BeNumerically(">", second))
			Expect(stderr).To(ContainSubstring("results exclude 3 workspace(s)"))
		})

		It("sorts global summary rows by commands, then tokens, then command name", func() {
			tmpDir := GinkgoT().TempDir()
			alphaPath := filepath.Join(tmpDir, "alpha.db")
			betaPath := filepath.Join(tmpDir, "beta.db")
			appendGainMetrics(alphaPath, []metrics.RunMetric{
				{Timestamp: time.Date(2026, 3, 26, 9, 0, 0, 0, time.UTC), Tool: "go", Command: "beta", RawBytes: 8, KeptBytes: 4},
				{Timestamp: time.Date(2026, 3, 26, 9, 1, 0, 0, time.UTC), Tool: "go", Command: "alpha", RawBytes: 8, KeptBytes: 4},
			})
			appendGainMetrics(betaPath, []metrics.RunMetric{
				{Timestamp: time.Date(2026, 3, 26, 9, 2, 0, 0, time.UTC), Tool: "go", Command: "beta", RawBytes: 4, KeptBytes: 4},
				{Timestamp: time.Date(2026, 3, 26, 9, 3, 0, 0, time.UTC), Tool: "go", Command: "alpha", RawBytes: 4, KeptBytes: 4},
				{Timestamp: time.Date(2026, 3, 26, 9, 4, 0, 0, time.UTC), Tool: "go", Command: "gamma", RawBytes: 16, KeptBytes: 8},
			})

			rows, err := queryGlobalSummaryRows(&globalQuerySession{
				sources: []globalMetricsSource{
					{CWD: "/repo-a", MetricsPath: alphaPath},
					{CWD: "/repo-b", MetricsPath: betaPath},
				},
				failures: map[string]globalQueryFailure{},
			}, metrics.QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(3))
			Expect(rows[0].Command).To(Equal("alpha"))
			Expect(rows[1].Command).To(Equal("beta"))
			Expect(rows[2].Command).To(Equal("gamma"))
		})

		It("prefers higher token totals before command-name ordering when global summary counts tie", func() {
			tmpDir := GinkgoT().TempDir()
			firstPath := filepath.Join(tmpDir, "first.db")
			secondPath := filepath.Join(tmpDir, "second.db")
			appendGainMetrics(firstPath, []metrics.RunMetric{
				{Timestamp: time.Date(2026, 3, 26, 9, 10, 0, 0, time.UTC), Tool: "go", Command: "beta", RawBytes: 20, KeptBytes: 4},
				{Timestamp: time.Date(2026, 3, 26, 9, 11, 0, 0, time.UTC), Tool: "go", Command: "alpha", RawBytes: 8, KeptBytes: 4},
			})
			appendGainMetrics(secondPath, []metrics.RunMetric{
				{Timestamp: time.Date(2026, 3, 26, 9, 12, 0, 0, time.UTC), Tool: "go", Command: "beta", RawBytes: 4, KeptBytes: 4},
				{Timestamp: time.Date(2026, 3, 26, 9, 13, 0, 0, time.UTC), Tool: "go", Command: "alpha", RawBytes: 4, KeptBytes: 4},
			})

			rows, err := queryGlobalSummaryRows(&globalQuerySession{
				sources: []globalMetricsSource{
					{CWD: "/repo-a", MetricsPath: firstPath},
					{CWD: "/repo-b", MetricsPath: secondPath},
				},
				failures: map[string]globalQueryFailure{},
			}, metrics.QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(2))
			Expect(rows[0].Command).To(Equal("beta"))
			Expect(rows[0].EstimatedInputTokens).To(BeNumerically(">", rows[1].EstimatedInputTokens))
			Expect(rows[1].Command).To(Equal("alpha"))
		})

		It("sorts global tool rows by commands, then tokens, then tool name", func() {
			tmpDir := GinkgoT().TempDir()
			firstPath := filepath.Join(tmpDir, "first.db")
			secondPath := filepath.Join(tmpDir, "second.db")
			appendGainMetrics(firstPath, []metrics.RunMetric{
				{Timestamp: time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC), Tool: "git", Command: "git status", RawBytes: 8, KeptBytes: 4},
				{Timestamp: time.Date(2026, 3, 26, 10, 1, 0, 0, time.UTC), Tool: "go", Command: "go test", RawBytes: 8, KeptBytes: 4},
			})
			appendGainMetrics(secondPath, []metrics.RunMetric{
				{Timestamp: time.Date(2026, 3, 26, 10, 2, 0, 0, time.UTC), Tool: "git", Command: "git diff", RawBytes: 4, KeptBytes: 4},
				{Timestamp: time.Date(2026, 3, 26, 10, 3, 0, 0, time.UTC), Tool: "go", Command: "go vet", RawBytes: 4, KeptBytes: 4},
				{Timestamp: time.Date(2026, 3, 26, 10, 4, 0, 0, time.UTC), Tool: "awk", Command: "awk", RawBytes: 16, KeptBytes: 8},
			})

			rows, err := queryGlobalSummaryToolRows(&globalQuerySession{
				sources: []globalMetricsSource{
					{CWD: "/repo-a", MetricsPath: firstPath},
					{CWD: "/repo-b", MetricsPath: secondPath},
				},
				failures: map[string]globalQueryFailure{},
			}, metrics.QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(3))
			Expect(rows[0].Tool).To(Equal("git"))
			Expect(rows[1].Tool).To(Equal("go"))
			Expect(rows[2].Tool).To(Equal("awk"))
		})

		It("prefers higher token totals before tool-name ordering when global tool counts tie", func() {
			tmpDir := GinkgoT().TempDir()
			firstPath := filepath.Join(tmpDir, "first.db")
			secondPath := filepath.Join(tmpDir, "second.db")
			appendGainMetrics(firstPath, []metrics.RunMetric{
				{Timestamp: time.Date(2026, 3, 26, 10, 10, 0, 0, time.UTC), Tool: "beta", Command: "one", RawBytes: 20, KeptBytes: 4},
				{Timestamp: time.Date(2026, 3, 26, 10, 11, 0, 0, time.UTC), Tool: "alpha", Command: "two", RawBytes: 8, KeptBytes: 4},
			})
			appendGainMetrics(secondPath, []metrics.RunMetric{
				{Timestamp: time.Date(2026, 3, 26, 10, 12, 0, 0, time.UTC), Tool: "beta", Command: "three", RawBytes: 4, KeptBytes: 4},
				{Timestamp: time.Date(2026, 3, 26, 10, 13, 0, 0, time.UTC), Tool: "alpha", Command: "four", RawBytes: 4, KeptBytes: 4},
			})

			rows, err := queryGlobalSummaryToolRows(&globalQuerySession{
				sources: []globalMetricsSource{
					{CWD: "/repo-a", MetricsPath: firstPath},
					{CWD: "/repo-b", MetricsPath: secondPath},
				},
				failures: map[string]globalQueryFailure{},
			}, metrics.QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(2))
			Expect(rows[0].Tool).To(Equal("beta"))
			Expect(rows[0].EstimatedInputTokens).To(BeNumerically(">", rows[1].EstimatedInputTokens))
			Expect(rows[1].Tool).To(Equal("alpha"))
		})

		It("merges global period rows by bucket and sorts by bucket start", func() {
			tmpDir := GinkgoT().TempDir()
			firstPath := filepath.Join(tmpDir, "first.db")
			secondPath := filepath.Join(tmpDir, "second.db")
			appendGainMetrics(firstPath, []metrics.RunMetric{
				{Timestamp: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), Tool: "go", Command: "one", RawBytes: 8, KeptBytes: 4},
				{Timestamp: time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC), Tool: "go", Command: "two", RawBytes: 8, KeptBytes: 4},
			})
			appendGainMetrics(secondPath, []metrics.RunMetric{
				{Timestamp: time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC), Tool: "go", Command: "three", RawBytes: 4, KeptBytes: 0},
				{Timestamp: time.Date(2026, 3, 2, 11, 0, 0, 0, time.UTC), Tool: "go", Command: "four", RawBytes: 4, KeptBytes: 0},
			})

			rows, err := queryGlobalPeriodRows(&globalQuerySession{
				sources: []globalMetricsSource{
					{CWD: "/repo-a", MetricsPath: firstPath},
					{CWD: "/repo-b", MetricsPath: secondPath},
				},
				failures: map[string]globalQueryFailure{},
			}, metrics.QueryOptions{Period: "day"})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(2))
			Expect(rows[0].BucketStart).To(Equal("2026-03-01"))
			Expect(rows[0].Commands).To(Equal(int64(2)))
			Expect(rows[1].BucketStart).To(Equal("2026-03-02"))
			Expect(rows[1].Commands).To(Equal(int64(2)))
		})

		It("sorts global period rows across more than two buckets in ascending bucket order", func() {
			tmpDir := GinkgoT().TempDir()
			firstPath := filepath.Join(tmpDir, "first.db")
			secondPath := filepath.Join(tmpDir, "second.db")
			appendGainMetrics(firstPath, []metrics.RunMetric{
				{Timestamp: time.Date(2026, 2, 28, 10, 0, 0, 0, time.UTC), Tool: "go", Command: "one", RawBytes: 8, KeptBytes: 4},
				{Timestamp: time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC), Tool: "go", Command: "two", RawBytes: 8, KeptBytes: 4},
			})
			appendGainMetrics(secondPath, []metrics.RunMetric{
				{Timestamp: time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC), Tool: "go", Command: "three", RawBytes: 4, KeptBytes: 0},
			})

			rows, err := queryGlobalPeriodRows(&globalQuerySession{
				sources: []globalMetricsSource{
					{CWD: "/repo-a", MetricsPath: firstPath},
					{CWD: "/repo-b", MetricsPath: secondPath},
				},
				failures: map[string]globalQueryFailure{},
			}, metrics.QueryOptions{Period: "day"})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(3))
			Expect(rows[0].BucketStart).To(Equal("2026-02-28"))
			Expect(rows[1].BucketStart).To(Equal("2026-03-01"))
			Expect(rows[2].BucketStart).To(Equal("2026-03-02"))
		})

		It("sorts global history rows by timestamp descending, then source, then command", func() {
			tmpDir := GinkgoT().TempDir()
			firstPath := filepath.Join(tmpDir, "first.db")
			secondPath := filepath.Join(tmpDir, "second.db")
			ts := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
			appendGainMetrics(firstPath, []metrics.RunMetric{
				{Timestamp: ts, Tool: "go", Command: "beta", RawBytes: 8, KeptBytes: 4},
				{Timestamp: ts.Add(time.Minute), Tool: "go", Command: "latest", RawBytes: 8, KeptBytes: 4},
			})
			appendGainMetrics(secondPath, []metrics.RunMetric{
				{Timestamp: ts, Tool: "go", Command: "alpha", RawBytes: 8, KeptBytes: 4},
			})

			rows, err := queryGlobalHistoryRows(&globalQuerySession{
				sources: []globalMetricsSource{
					{CWD: "/repo-b", MetricsPath: firstPath},
					{CWD: "/repo-a", MetricsPath: secondPath},
				},
				failures: map[string]globalQueryFailure{},
			}, metrics.QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(3))
			Expect(rows[0].Command).To(Equal("latest"))
			Expect(rows[1].Source).To(Equal("/repo-a"))
			Expect(rows[1].Command).To(Equal("alpha"))
			Expect(rows[2].Source).To(Equal("/repo-b"))
			Expect(rows[2].Command).To(Equal("beta"))
		})

		It("sorts global history rows by command when timestamp and source match", func() {
			tmpDir := GinkgoT().TempDir()
			path := filepath.Join(tmpDir, "history.db")
			ts := time.Date(2026, 3, 26, 13, 0, 0, 0, time.UTC)
			appendGainMetrics(path, []metrics.RunMetric{
				{Timestamp: ts, Tool: "go", Command: "beta", RawBytes: 8, KeptBytes: 4},
				{Timestamp: ts, Tool: "go", Command: "alpha", RawBytes: 8, KeptBytes: 4},
			})

			rows, err := queryGlobalHistoryRows(&globalQuerySession{
				sources:  []globalMetricsSource{{CWD: "/repo-a", MetricsPath: path}},
				failures: map[string]globalQueryFailure{},
			}, metrics.QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(2))
			Expect(rows[0].Source).To(Equal("/repo-a"))
			Expect(rows[0].Command).To(Equal("alpha"))
			Expect(rows[1].Command).To(Equal("beta"))
		})

		It("writes global history CSV rows with exact filters and derived metrics", func() {
			ts := time.Date(2026, 3, 26, 14, 15, 0, 0, time.UTC)
			out, err := captureStdout(func() error {
				return writeGlobalHistoryCSV([]globalHistoryRow{
					{
						HistoryRow: metrics.HistoryRow{
							Timestamp:             ts,
							Command:               "go test ./...",
							Tool:                  "go",
							DispatchKey:           "go:test",
							ExitCode:              3,
							Failed:                true,
							Passthrough:           false,
							DurationMS:            120,
							RawBytes:              8,
							KeptBytes:             4,
							DroppedBytes:          4,
							DropRatio:             0.5,
							EstimatedInputTokens:  2,
							EstimatedOutputTokens: 1,
							EstimatedSavedTokens:  1,
							EstimatedSavingsPct:   50,
						},
						Source: "/repo-a",
					},
				}, filtersEnvelope{Since: "7d", Tool: "go", Failed: true})
			})
			Expect(err).NotTo(HaveOccurred())

			records, err := csv.NewReader(strings.NewReader(out)).ReadAll()
			Expect(err).NotTo(HaveOccurred())
			Expect(records).To(HaveLen(2))
			Expect(records[0]).To(Equal([]string{
				"dataset", "period", "since", "tool_filter", "failed_filter", "row_kind",
				"timestamp", "source", "command", "tool", "dispatch_key", "exit_code", "failed", "passthrough", "duration_ms",
				"commands", "raw_bytes", "kept_bytes", "dropped_bytes", "drop_ratio",
				"estimated_input_tokens", "estimated_output_tokens", "estimated_saved_tokens", "estimated_savings_pct",
			}))
			Expect(records[1]).To(Equal([]string{
				"history", "", "7d", "go", "true", "data",
				ts.Format(time.RFC3339), "/repo-a", "go test ./...", "go", "go:test", "3", "true", "false", "120",
				"", "8", "4", "4", "0.5000", "2", "1", "1", "50.00",
			}))
		})

		DescribeTable("local token and tail helpers",
			func(bytes int64, input string, max int, wantTokens int64, wantTail string) {
				Expect(localTokensFromBytes(bytes)).To(Equal(wantTokens))
				Expect(truncateTailForDisplay(input, max)).To(Equal(wantTail))
			},
			Entry("clamps negative bytes and trims whitespace-free empty tails", int64(-1), "abcdef", 0, int64(0), ""),
			Entry("clamps zero bytes and non-positive tails", int64(0), "abcdef", 0, int64(0), ""),
			Entry("keeps a single byte at one token", int64(1), "abcdef", 6, int64(1), "abcdef"),
			Entry("keeps exact four-byte boundaries at one token", int64(4), "abcdef", 6, int64(1), "abcdef"),
			Entry("rounds token counts up in groups of four and keeps short tails", int64(5), "abcdef", 6, int64(2), "abcdef"),
			Entry("uses raw tail slices when max is tiny", int64(8), "abcdef", 3, int64(2), "def"),
			Entry("prefixes long tails with an ellipsis when space allows", int64(9), "abcdef", 5, int64(3), "...ef"),
		)

		It("fills local derived fields consistently across summary, total, and period rows", func() {
			summaryRow := metrics.SummaryRow{RawBytes: 8, KeptBytes: 4}
			fillLocalSummaryRowDerived(&summaryRow)
			Expect(summaryRow.DroppedBytes).To(Equal(int64(4)))
			Expect(summaryRow.DropRatio).To(Equal(0.5))
			Expect(summaryRow.EstimatedInputTokens).To(Equal(int64(2)))
			Expect(summaryRow.EstimatedOutputTokens).To(Equal(int64(1)))
			Expect(summaryRow.EstimatedSavedTokens).To(Equal(int64(1)))
			Expect(summaryRow.EstimatedSavingsPct).To(Equal(50.0))

			toolRow := metrics.SummaryToolRow{RawBytes: 8, KeptBytes: 4}
			fillLocalSummaryToolDerived(&toolRow)
			Expect(toolRow.DroppedBytes).To(Equal(int64(4)))
			Expect(toolRow.DropRatio).To(Equal(0.5))
			Expect(toolRow.EstimatedInputTokens).To(Equal(int64(2)))
			Expect(toolRow.EstimatedOutputTokens).To(Equal(int64(1)))
			Expect(toolRow.EstimatedSavedTokens).To(Equal(int64(1)))
			Expect(toolRow.EstimatedSavingsPct).To(Equal(50.0))

			total := metrics.SummaryTotal{RawBytes: 8, KeptBytes: 4}
			fillLocalSummaryTotalDerived(&total)
			Expect(total.DroppedBytes).To(Equal(int64(4)))
			Expect(total.DropRatio).To(Equal(0.5))
			Expect(total.EstimatedInputTokens).To(Equal(int64(2)))
			Expect(total.EstimatedOutputTokens).To(Equal(int64(1)))
			Expect(total.EstimatedSavedTokens).To(Equal(int64(1)))
			Expect(total.EstimatedSavingsPct).To(Equal(50.0))

			periodRow := metrics.PeriodRow{RawBytes: 8, KeptBytes: 4}
			fillLocalPeriodRowDerived(&periodRow)
			Expect(periodRow.DroppedBytes).To(Equal(int64(4)))
			Expect(periodRow.DropRatio).To(Equal(0.5))
			Expect(periodRow.EstimatedInputTokens).To(Equal(int64(2)))
			Expect(periodRow.EstimatedOutputTokens).To(Equal(int64(1)))
			Expect(periodRow.EstimatedSavedTokens).To(Equal(int64(1)))
			Expect(periodRow.EstimatedSavingsPct).To(Equal(50.0))
		})

		It("keeps local derived fields at zero when raw bytes are zero", func() {
			summaryRow := metrics.SummaryRow{}
			fillLocalSummaryRowDerived(&summaryRow)
			Expect(summaryRow).To(Equal(metrics.SummaryRow{}))

			toolRow := metrics.SummaryToolRow{}
			fillLocalSummaryToolDerived(&toolRow)
			Expect(toolRow).To(Equal(metrics.SummaryToolRow{}))

			total := metrics.SummaryTotal{}
			fillLocalSummaryTotalDerived(&total)
			Expect(total).To(Equal(metrics.SummaryTotal{}))

			periodRow := metrics.PeriodRow{}
			fillLocalPeriodRowDerived(&periodRow)
			Expect(periodRow).To(Equal(metrics.PeriodRow{}))
		})

		It("aggregates summary totals before deriving shared metrics", func() {
			total := totalFromSummaryRows([]metrics.SummaryRow{
				{Command: "go test", Commands: 2, RawBytes: 8, KeptBytes: 4},
				{Command: "git status", Commands: 1, RawBytes: 4, KeptBytes: 4},
			})

			Expect(total.Commands).To(Equal(int64(3)))
			Expect(total.RawBytes).To(Equal(int64(12)))
			Expect(total.KeptBytes).To(Equal(int64(8)))
			Expect(total.DroppedBytes).To(Equal(int64(4)))
			Expect(total.DropRatio).To(BeNumerically("~", 4.0/12.0, 1e-12))
			Expect(total.EstimatedInputTokens).To(Equal(int64(3)))
			Expect(total.EstimatedOutputTokens).To(Equal(int64(2)))
			Expect(total.EstimatedSavedTokens).To(Equal(int64(1)))
			Expect(total.EstimatedSavingsPct).To(BeNumerically("~", (1.0/3.0)*100, 1e-12))
		})
	})
})

func appendGainMetrics(path string, seed []metrics.RunMetric) {
	for _, m := range seed {
		Expect(metrics.Append(path, m)).To(Succeed())
	}
}

func appendGlobalWorkspaceMetrics(home, cwd string, seed []metrics.RunMetric) {
	path := filepath.Join(cwd, ".ccp", "gain.db")
	appendGainMetrics(path, seed)
	Expect(workspaces.UpsertPath(workspaces.PathForHome(home), cwd, path)).To(Succeed())
}

func captureStdout(run func() error) (string, error) {
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w
	defer func() {
		os.Stdout = orig
	}()

	runErr := run()
	closeErr := w.Close()

	var buf bytes.Buffer
	_, copyErr := io.Copy(&buf, r)
	readCloseErr := r.Close()

	return buf.String(), errors.Join(runErr, closeErr, copyErr, readCloseErr)
}
