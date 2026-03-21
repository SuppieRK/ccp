package lifecycle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
			Expect(out).To(ContainSubstring("2 cmds · 425 → 225 tokens (47.1% saved)"))
			Expect(out).To(ContainSubstring("Wins  : go 66.7%"))
			Expect(out).To(ContainSubstring("Drag  : git (1 cmds)"))
			Expect(out).To(ContainSubstring("Trend : insufficient data"))
		})

		It("renders the compact gain table", func() {
			out := runGain(flagTable)
			Expect(out).To(ContainSubstring("2 cmds · 425 → 225 tokens (47.1% saved)"))
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
			Expect(out).To(ContainSubstring("4 cmds · 850 → 450 tokens (47.1% saved) [global]"))
			Expect(out).To(ContainSubstring("Wins  : go 66.7%"))
			Expect(out).To(ContainSubstring("Drag  : git (2 cmds)"))
			Expect(out).To(ContainSubstring("go"))
			Expect(out).To(ContainSubstring("git"))
		})

		It("ignores unreadable registered workspaces during global gain aggregation", func() {
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)

			repoOne := filepath.Join(tmpDir, "repo-one")
			appendGlobalWorkspaceMetrics(home, repoOne, []metrics.RunMetric{
				{Timestamp: time.Now().UTC().Add(-2 * time.Hour), Tool: "go", Command: "go test ./...", RawBytes: 1200, KeptBytes: 400},
			})
			Expect(workspaces.Upsert(filepath.Join(tmpDir, "missing-repo"), filepath.Join(tmpDir, "missing-repo", ".ccp", "gain.db"))).To(Succeed())

			out := runGain(flagGlobal, flagFormat, "json")
			Expect(out).To(ContainSubstring(`"dataset": "summary"`))
			Expect(out).To(ContainSubstring(`"go test ./..."`))
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
			Expect(textOut).To(ContainSubstring("[period=week global]"))
			Expect(textOut).To(ContainSubstring("Wins  :"))
			Expect(textOut).To(ContainSubstring("Drag  :"))
			Expect(textOut).To(ContainSubstring("Trend :"))

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
			Expect(out).To(ContainSubstring("5,002,000"))
			Expect(out).To(ContainSubstring("27,000"))
			Expect(out).To(ContainSubstring("Wins  : gradle 99.5%"))
			Expect(out).To(ContainSubstring("Drag  : jar (1 cmds)"))
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
			Expect(periodText).To(ContainSubstring("[period=day]"))
			Expect(periodText).To(ContainSubstring("Wins  :"))
			Expect(periodText).To(ContainSubstring("Drag  :"))
			Expect(periodText).To(ContainSubstring("Trend :"))

			periodTable := runGain(flagPeriod, "day", flagTable)
			Expect(periodTable).To(ContainSubstring("ccp gain (estimated tokens: 4B/token)"))
			Expect(periodTable).To(ContainSubstring("BUCKET"))
			Expect(periodTable).To(ContainSubstring("period=day"))

			periodCSV := runGain(flagPeriod, "week", flagFormat, "csv")
			Expect(periodCSV).To(ContainSubstring("dataset,period,since,tool_filter,failed_filter,bucket"))
			Expect(periodCSV).To(ContainSubstring("period,week"))
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
			Expect(gainOut).To(ContainSubstring("0 cmds · 0 → 0 tokens (0.0% saved)"))
			Expect(gainOut).NotTo(ContainSubstring("filters:"))
			Expect(gainOut).To(ContainSubstring(noResultsMsg))
		})
	})

	Context("when validating flags", func() {
		It("rejects invalid gain flags", func() {
			Expect(RunGain([]string{flagFormat, "xml"}, path)).To(HaveOccurred())
			Expect(RunGain([]string{flagSince, "nope"}, path)).To(HaveOccurred())
			Expect(RunGain([]string{flagSince, "2d"}, path)).To(Succeed())
			Expect(RunGain([]string{flagSince, "1w"}, path)).To(Succeed())
			Expect(RunGain([]string{flagFormat, "json", flagTable}, path)).To(HaveOccurred())
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
		})
	})
})

var _ = Describe("gain formatting helpers", func() {
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

	It("formats trend summaries with one decimal precision", func() {
		rows := []metrics.PeriodRow{
			{BucketStart: "2026-03-01", EstimatedSavingsPct: 31.4},
			{BucketStart: "2026-03-02", EstimatedSavingsPct: 31.4},
			{BucketStart: "2026-03-03", EstimatedSavingsPct: 37.6},
			{BucketStart: "2026-03-04", EstimatedSavingsPct: 37.6},
		}
		Expect(trendSummaryText(rows, "week")).To(Equal("↑ +6.2 pts WoW (31.4% → 37.6%) · clear gain"))
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

		Expect(trendSummaryText(flatRows, "week")).To(Equal("→ flat WoW (52.0% → 52.0%) · holding high"))
		Expect(trendSummaryText(downRows, "week")).To(Equal("↓ -2.1 pts WoW (13.0% → 10.9%) · slipping"))
	})

	DescribeTable("truncateForDisplay branches",
		func(input string, max int, want string) {
			Expect(truncateForDisplay(input, max)).To(Equal(want))
		},
		Entry("max<=0", "abcdef", 0, ""),
		Entry("max<=3", "abcdef", 3, "abc"),
		Entry("ellipsis", "abcdef", 5, "ab..."),
	)
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
