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

	"github.com/SuppieRK/cmdshape/internal/metrics"
	"github.com/SuppieRK/cmdshape/internal/workspaces"
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

		It("clamps anomalous global kept bytes to canonical derived totals", func() {
			home := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(home, nil)
			DeferCleanup(restore)
			Expect(os.Remove(path)).To(Succeed())

			repo := filepath.Join(tmpDir, "repo-clamped-global")
			appendGlobalWorkspaceMetrics(home, repo, []metrics.RunMetric{
				{Timestamp: time.Now().UTC().Add(-time.Hour), Tool: "go", Command: "go test ./...", RawBytes: 8, KeptBytes: 42},
			})

			out := runGain(flagGlobal, flagFormat, "json")

			var env summaryEnvelope
			Expect(json.Unmarshal([]byte(out), &env)).To(Succeed())
			Expect(env.Storage.Observed).To(Equal(1))
			Expect(env.Storage.Pending).To(BeZero())
			Expect(env.Storage.StorageErrors).To(BeZero())
			Expect(env.Total.RawBytes).To(Equal(int64(8)))
			Expect(env.Total.KeptBytes).To(Equal(int64(42)))
			Expect(env.Total.DroppedBytes).To(Equal(int64(0)))
			Expect(env.Total.DropRatio).To(Equal(0.0))
			Expect(env.Total.EstimatedInputTokens).To(Equal(int64(2)))
			Expect(env.Total.EstimatedOutputTokens).To(Equal(int64(2)))
			Expect(env.Total.EstimatedSavedTokens).To(Equal(int64(0)))
			Expect(env.Total.EstimatedSavingsPct).To(Equal(0.0))
			Expect(env.Rows).To(HaveLen(1))
			Expect(env.Rows[0].DroppedBytes).To(Equal(int64(0)))
			Expect(env.Rows[0].EstimatedOutputTokens).To(Equal(int64(2)))
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
			corruptMetricsPath := filepath.Join(corruptRepo, ".cmdshape", "gain.db")
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
			Expect(stderr).To(ContainSubstring("cmdshape gain --global: warning: skipped workspace"))
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
			repoMetricsPath := filepath.Join(repoRoot, ".cmdshape", "gain.db")
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
			repoMetricsPath := filepath.Join(repoRoot, ".cmdshape", "gain.db")
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
			repoMetricsPath := filepath.Join(repoRoot, ".cmdshape", "gain.db")
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
			Expect(textOut).To(ContainSubstring("cmdshape history [global]"))
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
			repoMetricsPath := filepath.Join(repoRoot, ".cmdshape", "gain.db")
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
			repoMetricsPath := filepath.Join(repoRoot, ".cmdshape", "gain.db")
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
			corruptMetricsPath := filepath.Join(corruptRepo, ".cmdshape", "gain.db")
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
			Expect(stderr).To(ContainSubstring("cmdshape history --global: warning: skipped workspace"))
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
			Expect(globalOut).To(ContainSubstring("cmdshape history [global]"))
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
			Expect(env.Storage.Observed).To(Equal(2))

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

	Context("when purging history", func() {
		It("requires confirmation and removes only records older than the cutoff", func() {
			Expect(RunHistory([]string{"purge", "--before", "90m"}, path)).To(MatchError(ContainSubstring("requires --yes")))

			out := runHistory("purge", "--before", "90m", "--yes")

			Expect(out).To(ContainSubstring("Purged 1 history records"))
			rows, err := metrics.QueryHistory(path, metrics.QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(1))
			Expect(rows[0].Tool).To(Equal("git"))
		})

		DescribeTable("rejects invalid purge invocations",
			func(args []string, message string) {
				Expect(RunHistory(append([]string{"purge"}, args...), path)).To(MatchError(ContainSubstring(message)))
			},
			Entry("without a cutoff", []string{"--yes"}, "requires --before"),
			Entry("with a zero cutoff", []string{"--before", "0h", "--yes"}, "invalid --before"),
			Entry("with an invalid cutoff", []string{"--before", "later", "--yes"}, "invalid --before"),
			Entry("with positional arguments", []string{"--before", "1h", "--yes", "extra"}, "does not accept positional"),
		)
	})

	Context("when the history database is empty", func() {
		BeforeEach(func() {
			Expect(os.Remove(path)).To(Succeed())
		})

		It("includes filters and no-results markers in text output", func() {
			out := runHistory(flagFormat, "text")
			Expect(out).To(ContainSubstring("cmdshape history"))
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
			Entry("prefers the workspace cwd", globalQueryFailure{CWD: "/repo", MetricsPath: "/repo/.cmdshape/gain.db"}, "/repo"),
			Entry("falls back to the metrics path", globalQueryFailure{MetricsPath: "/repo/.cmdshape/gain.db"}, "/repo/.cmdshape/gain.db"),
			Entry("uses an explicit unknown placeholder when no source exists", globalQueryFailure{}, "<unknown>"),
		)

		It("records failures once and falls back to cwd keys when the metrics path is empty", func() {
			session := &globalQuerySession{failures: map[string]globalQueryFailure{}}

			session.recordFailure(globalMetricsSource{CWD: "/repo-a"}, errors.New("first"))
			session.recordFailure(globalMetricsSource{CWD: "/repo-a"}, errors.New("second"))
			session.recordFailure(globalMetricsSource{CWD: "/repo-b", MetricsPath: "/repo-b/.cmdshape/gain.db"}, nil)

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
			registeredPath := filepath.Join(registeredRepo, ".cmdshape", "gain.db")
			missingPath := filepath.Join(missingRepo, ".cmdshape", "gain.db")
			currentPath := filepath.Join(currentRepo, ".cmdshape", "gain.db")

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
				sources, err = globalMetricsSources(filepath.Join(currentRepo, ".cmdshape", ".", "gain.db"))
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
			Entry("normalizes a relative metrics path using the current working directory", filepath.Join(".", ".cmdshape", "gain.db"), false),
		)

		It("writes global warnings in cwd-then-metrics-path order", func() {
			session := &globalQuerySession{
				failures: map[string]globalQueryFailure{
					"b": {CWD: "/repo-b", MetricsPath: "/repo-b/.cmdshape/gain.db", Err: errors.New("broken b")},
					"z": {CWD: "/repo-a", MetricsPath: "/repo-a/.cmdshape/z.db", Err: errors.New("broken z")},
					"a": {CWD: "/repo-a", MetricsPath: "/repo-a/.cmdshape/a.db", Err: errors.New("broken a")},
				},
			}

			stderr, err := captureStderrOutput(func() error {
				session.writeWarnings("gain")
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			first := strings.Index(stderr, "/repo-a (/repo-a/.cmdshape/a.db): broken a")
			second := strings.Index(stderr, "/repo-a (/repo-a/.cmdshape/z.db): broken z")
			third := strings.Index(stderr, "/repo-b (/repo-b/.cmdshape/gain.db): broken b")
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

		It("clamps local derived fields when kept bytes exceed raw bytes", func() {
			summaryRow := metrics.SummaryRow{RawBytes: 8, KeptBytes: 42}
			fillLocalSummaryRowDerived(&summaryRow)
			Expect(summaryRow.DroppedBytes).To(Equal(int64(0)))
			Expect(summaryRow.DropRatio).To(Equal(0.0))
			Expect(summaryRow.EstimatedInputTokens).To(Equal(int64(2)))
			Expect(summaryRow.EstimatedOutputTokens).To(Equal(int64(2)))
			Expect(summaryRow.EstimatedSavedTokens).To(Equal(int64(0)))
			Expect(summaryRow.EstimatedSavingsPct).To(Equal(0.0))

			toolRow := metrics.SummaryToolRow{RawBytes: 8, KeptBytes: 42}
			fillLocalSummaryToolDerived(&toolRow)
			Expect(toolRow.DroppedBytes).To(Equal(int64(0)))
			Expect(toolRow.EstimatedOutputTokens).To(Equal(int64(2)))

			total := metrics.SummaryTotal{RawBytes: 8, KeptBytes: 42}
			fillLocalSummaryTotalDerived(&total)
			Expect(total.DroppedBytes).To(Equal(int64(0)))
			Expect(total.EstimatedOutputTokens).To(Equal(int64(2)))

			periodRow := metrics.PeriodRow{RawBytes: 8, KeptBytes: 42}
			fillLocalPeriodRowDerived(&periodRow)
			Expect(periodRow.DroppedBytes).To(Equal(int64(0)))
			Expect(periodRow.EstimatedOutputTokens).To(Equal(int64(2)))
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
	path := filepath.Join(cwd, ".cmdshape", "gain.db")
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
