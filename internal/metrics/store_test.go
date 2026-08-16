package metrics

import (
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	bolt "go.etcd.io/bbolt"
)

const (
	metricsGoTestCommand = "go test ./..."
	gitignoreFileName    = ".gitignore"
	gainDBFileName       = "gain.db"
)

type derivedExpectation struct {
	dropped      int64
	ratio        float64
	inputTokens  int64
	outputTokens int64
	savedTokens  int64
	savingsPct   float64
}

var _ = Describe("metrics storage", func() {
	var tempDir string

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
	})

	It("appends metrics and loads a summary", func() {
		path := filepath.Join(tempDir, "metrics", "runs.db")

		Expect(Append(path, RunMetric{
			Tool:      "go",
			Command:   metricsGoTestCommand,
			RawBytes:  10,
			KeptBytes: 4,
			ExitCode:  0,
		})).To(Succeed())
		Expect(Append(path, RunMetric{
			Tool:      "git",
			Command:   "git status",
			RawBytes:  6,
			KeptBytes: 3,
			ExitCode:  1,
		})).To(Succeed())

		got, err := LoadSummary(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Runs).To(Equal(2))
		Expect(got.RawLines).To(Equal(16))
		Expect(got.KeptLines).To(Equal(7))
		Expect(got.Dropped).To(Equal(9))
		Expect(got.DropRatio).To(Equal(9.0 / 16.0))
	})

	DescribeTable("loading a zero summary",
		func(path string) {
			if path == "__missing__" {
				path = filepath.Join(tempDir, "missing.db")
			}
			got, err := LoadSummary(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(Summary{}))
		},
		Entry("for a missing file", "__missing__"),
		Entry("for an empty path", ""),
	)

	It("noops append with an empty path", func() {
		Expect(Append("", RunMetric{Tool: "noop", RawBytes: 1, KeptBytes: 1})).To(Succeed())
	})

	It("fails when the parent path is a file", func() {
		parentFile := filepath.Join(tempDir, "not-a-dir")
		Expect(os.WriteFile(parentFile, []byte("x"), 0o644)).To(Succeed())

		err := Append(filepath.Join(parentFile, "metrics.db"), RunMetric{Tool: "go", RawBytes: 1, KeptBytes: 1})
		Expect(err).To(HaveOccurred())
	})

	It("fails when the target path is a directory", func() {
		err := Append(tempDir, RunMetric{Tool: "go", RawBytes: 1, KeptBytes: 1})
		Expect(err).To(HaveOccurred())
	})

	Describe("contained project appends", func() {
		It("writes and reads a normal project database", func() {
			project := filepath.Join(tempDir, "project")
			Expect(os.Mkdir(project, 0o755)).To(Succeed())
			path := filepath.Join(project, ".cmdshape", gainDBFileName)

			Expect(AppendProject(project, path, RunMetric{
				Tool:      "go",
				Command:   metricsGoTestCommand,
				RawBytes:  10,
				KeptBytes: 4,
			})).To(Succeed())

			history, err := QueryHistory(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(HaveLen(1))
		})

		It("rejects a dangling database symlink without creating its target", func() {
			project := filepath.Join(tempDir, "project")
			cmdshapeDir := filepath.Join(project, ".cmdshape")
			Expect(os.MkdirAll(cmdshapeDir, 0o755)).To(Succeed())
			outside := filepath.Join(tempDir, "outside.db")
			path := filepath.Join(cmdshapeDir, gainDBFileName)
			if err := os.Symlink(outside, path); err != nil {
				Skip("symlink creation unavailable: " + err.Error())
			}

			err := AppendProject(project, path, RunMetric{Tool: "go", RawBytes: 1, KeptBytes: 1})

			Expect(err).To(HaveOccurred())
			Expect(outside).NotTo(BeAnExistingFile())
		})

		It("rejects a symlinked project state directory", func() {
			project := filepath.Join(tempDir, "project")
			Expect(os.Mkdir(project, 0o755)).To(Succeed())
			outside := filepath.Join(tempDir, "outside")
			Expect(os.Mkdir(outside, 0o755)).To(Succeed())
			if err := os.Symlink(outside, filepath.Join(project, ".cmdshape")); err != nil {
				Skip("symlink creation unavailable: " + err.Error())
			}

			err := AppendProject(project, filepath.Join(project, ".cmdshape", gainDBFileName), RunMetric{Tool: "go", RawBytes: 1, KeptBytes: 1})

			Expect(err).To(HaveOccurred())
			Expect(filepath.Join(outside, gainDBFileName)).NotTo(BeAnExistingFile())
		})

		It("rejects a metrics path outside the project root", func() {
			project := filepath.Join(tempDir, "project")
			Expect(os.Mkdir(project, 0o755)).To(Succeed())
			outside := filepath.Join(tempDir, "outside.db")

			err := AppendProject(project, outside, RunMetric{Tool: "go", RawBytes: 1, KeptBytes: 1})

			Expect(err).To(HaveOccurred())
			Expect(outside).NotTo(BeAnExistingFile())
		})

		It("consolidates 100 concurrent durable spool events without loss", func() {
			project := filepath.Join(tempDir, "project")
			Expect(os.Mkdir(project, 0o755)).To(Succeed())
			path := filepath.Join(project, ".cmdshape", gainDBFileName)

			var wg sync.WaitGroup
			errs := make(chan error, 100)
			for index := range 100 {
				wg.Go(func() {
					errs <- AppendProject(project, path, RunMetric{
						Timestamp: time.Now().UTC(),
						Command:   "go test ./pkg/" + strconv.Itoa(index),
						Tool:      "go",
						RawBytes:  10,
						KeptBytes: 5,
					})
				})
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(consolidateProjectSpool(project, path)).To(Succeed())

			history, err := QueryHistory(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(HaveLen(100))
			status := ProjectStorageStatus(project, path)
			Expect(status.Observed).To(Equal(100))
			Expect(status.Pending).To(BeZero())
		})

		It("leaves a pending event while Bolt is locked and commits it later", func() {
			project := filepath.Join(tempDir, "project")
			Expect(os.Mkdir(project, 0o755)).To(Succeed())
			path := filepath.Join(project, ".cmdshape", gainDBFileName)
			Expect(AppendProject(project, path, RunMetric{Tool: "seed", RawBytes: 1, KeptBytes: 1})).To(Succeed())

			db, err := openDBAt(project, path, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(AppendProject(project, path, RunMetric{Tool: "pending", RawBytes: 2, KeptBytes: 1})).To(Succeed())
			Expect(ProjectStorageStatus(project, path).Pending).To(Equal(1))
			Expect(db.Close()).To(Succeed())

			Expect(consolidateProjectSpool(project, path)).To(Succeed())
			history, err := QueryHistory(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(HaveLen(2))
			Expect(ProjectStorageStatus(project, path).Pending).To(BeZero())
		})

		It("consolidates pending events before purging the requested cutoff", func() {
			project := filepath.Join(tempDir, "project")
			Expect(os.Mkdir(project, 0o755)).To(Succeed())
			path := filepath.Join(project, ".cmdshape", gainDBFileName)
			cutoff := time.Now().UTC().Truncate(time.Second)
			Expect(AppendProject(project, path, RunMetric{
				Timestamp: cutoff.Add(time.Second),
				Tool:      "retained",
				RawBytes:  1,
				KeptBytes: 1,
			})).To(Succeed())

			db, err := openDBAt(project, path, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(AppendProject(project, path, RunMetric{
				Timestamp: cutoff.Add(-time.Second),
				Tool:      "pending-old",
				RawBytes:  2,
				KeptBytes: 1,
			})).To(Succeed())
			Expect(ProjectStorageStatus(project, path).Pending).To(Equal(1))
			Expect(db.Close()).To(Succeed())

			removed, err := PurgeProjectBefore(project, path, cutoff)

			Expect(err).NotTo(HaveOccurred())
			Expect(removed).To(Equal(1))
			Expect(ProjectStorageStatus(project, path).Pending).To(BeZero())
			history, err := QueryHistory(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(HaveLen(1))
			Expect(history[0].Tool).To(Equal("retained"))
		})

		It("commits duplicate spool event IDs exactly once", func() {
			project := filepath.Join(tempDir, "project")
			Expect(os.Mkdir(project, 0o755)).To(Succeed())
			path := filepath.Join(project, ".cmdshape", gainDBFileName)
			Expect(ensureSchema(project, path)).To(Succeed())
			db, err := openDBAt(project, path, false)
			Expect(err).NotTo(HaveOccurred())
			event := spoolEvent{
				ID:     "duplicate-event",
				Record: normalizeMetric(RunMetric{Tool: "go", RawBytes: 2, KeptBytes: 1}),
			}
			Expect(commitSpoolEvent(db, event)).To(Succeed())
			Expect(commitSpoolEvent(db, event)).To(Succeed())
			Expect(db.Close()).To(Succeed())

			history, err := QueryHistory(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(HaveLen(1))
		})

		It("bounds exactly-once markers with the same retention policy", func() {
			project := filepath.Join(tempDir, "project")
			Expect(os.Mkdir(project, 0o755)).To(Succeed())
			path := filepath.Join(project, ".cmdshape", gainDBFileName)
			Expect(ensureSchema(project, path)).To(Succeed())
			db, err := openDBAt(project, path, false)
			Expect(err).NotTo(HaveOccurred())

			oldTimestamp := time.Now().UTC().Add(-defaultRetention - time.Hour).Unix()
			recentTimestamp := time.Now().UTC().Unix()
			Expect(db.Update(func(tx *bolt.Tx) error {
				events := tx.Bucket(eventsBucket)
				if err := events.Put([]byte("old"), encodeRunKey(oldTimestamp, 0)[:8]); err != nil {
					return err
				}
				if err := events.Put([]byte("recent"), encodeRunKey(recentTimestamp, 0)[:8]); err != nil {
					return err
				}
				return pruneOldEventIDs(events, time.Now().UTC().Add(-defaultRetention), pruneBatchLimit)
			})).To(Succeed())
			Expect(db.View(func(tx *bolt.Tx) error {
				events := tx.Bucket(eventsBucket)
				Expect(events.Get([]byte("old"))).To(BeNil())
				Expect(events.Get([]byte("recent"))).NotTo(BeNil())
				return nil
			})).To(Succeed())
			Expect(db.Close()).To(Succeed())
		})

		It("uses private modes for automatic metrics state", func() {
			project := filepath.Join(tempDir, "project")
			Expect(os.Mkdir(project, 0o755)).To(Succeed())
			path := filepath.Join(project, ".cmdshape", gainDBFileName)

			Expect(AppendProject(project, path, RunMetric{Tool: "go", RawBytes: 1, KeptBytes: 1})).To(Succeed())

			cmdshapeInfo, err := os.Stat(filepath.Join(project, ".cmdshape"))
			Expect(err).NotTo(HaveOccurred())
			spoolInfo, err := os.Stat(filepath.Join(project, ".cmdshape", spoolDirectoryName))
			Expect(err).NotTo(HaveOccurred())
			dbInfo, err := os.Stat(path)
			Expect(err).NotTo(HaveOccurred())
			if runtime.GOOS != "windows" {
				Expect(cmdshapeInfo.Mode().Perm()).To(Equal(os.FileMode(0o700)))
				Expect(spoolInfo.Mode().Perm()).To(Equal(os.FileMode(0o700)))
				Expect(dbInfo.Mode().Perm()).To(Equal(os.FileMode(0o600)))
			}
		})
	})

	It("purges records strictly before the requested cutoff", func() {
		path := filepath.Join(tempDir, "metrics.db")
		cutoff := time.Now().UTC().Truncate(time.Second)
		appendRunMetrics(path,
			RunMetric{Timestamp: cutoff.Add(-time.Second), Tool: "old", RawBytes: 1, KeptBytes: 1},
			RunMetric{Timestamp: cutoff, Tool: "boundary", RawBytes: 1, KeptBytes: 1},
			RunMetric{Timestamp: cutoff.Add(time.Second), Tool: "new", RawBytes: 1, KeptBytes: 1},
		)

		removed, err := PurgeBefore(path, cutoff)
		Expect(err).NotTo(HaveOccurred())
		Expect(removed).To(Equal(1))
		history, err := QueryHistory(path, QueryOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(HaveLen(2))
		Expect([]string{history[0].Tool, history[1].Tool}).To(ConsistOf("boundary", "new"))
	})

	It("opens writable metrics databases with durable sync enabled", func() {
		path := filepath.Join(tempDir, "metrics.db")

		db, err := openDB(path, false)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(db.Close()).To(Succeed())
		})

		Expect(db.NoSync).To(BeFalse())
	})

	It("aborts queries when a stored run record is corrupt", func() {
		path := filepath.Join(tempDir, "metrics.db")
		Expect(Append(path, RunMetric{Tool: "go", Command: "valid", RawBytes: 1, KeptBytes: 1})).To(Succeed())
		db, err := openDB(path, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(db.Update(func(tx *bolt.Tx) error {
			return tx.Bucket(runsBucket).Put([]byte("corrupt-key"), []byte{1, 2, 3})
		})).To(Succeed())
		Expect(db.Close()).To(Succeed())

		rows, err := QuerySummaryRows(path, QueryOptions{})
		Expect(err).To(MatchError(ContainSubstring("decode run record")))
		Expect(rows).To(BeNil())
	})

	It("repairs an existing database missing the metrics bucket", func() {
		path := filepath.Join(tempDir, "metrics.db")

		db, err := bolt.Open(path, 0o600, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(db.Close()).To(Succeed())

		Expect(Append(path, RunMetric{
			Tool:      "go",
			Command:   metricsGoTestCommand,
			RawBytes:  10,
			KeptBytes: 4,
			ExitCode:  0,
		})).To(Succeed())

		summary, err := LoadSummary(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(summary.Runs).To(Equal(1))
		Expect(summary.RawLines).To(Equal(10))
		Expect(summary.KeptLines).To(Equal(4))
	})

	It("truncates long command text deterministically", func() {
		path := filepath.Join(tempDir, "metrics.db")
		long := strings.Repeat("x", 2000)

		Expect(Append(path, RunMetric{
			Tool:      "go",
			Command:   long,
			RawBytes:  100,
			KeptBytes: 25,
			ExitCode:  0,
		})).To(Succeed())

		history, err := QueryHistory(path, QueryOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(HaveLen(1))
		Expect(len([]rune(history[0].Command))).To(Equal(1024))
		Expect(history[0].Command).To(HaveSuffix("..."))
	})

	It("preserves negative exit codes in failed history queries", func() {
		path := filepath.Join(tempDir, "metrics.db")

		Expect(Append(path, RunMetric{
			Tool:      "node",
			Command:   "node crashed.js",
			RawBytes:  12,
			KeptBytes: 12,
			ExitCode:  -1,
		})).To(Succeed())

		failedHistory, err := QueryHistory(path, QueryOptions{Failed: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(failedHistory).To(HaveLen(1))
		Expect(failedHistory[0].ExitCode).To(Equal(-1))
		Expect(failedHistory[0].Failed).To(BeTrue())
	})

	It("preserves dispatch keys and durations in history rows", func() {
		path := filepath.Join(tempDir, "metrics.db")

		Expect(Append(path, RunMetric{
			Tool:        "go",
			Command:     metricsGoTestCommand,
			Dispatch:    "go:test",
			RawBytes:    18,
			KeptBytes:   6,
			DurationMS:  275,
			Passthrough: true,
		})).To(Succeed())

		history, err := QueryHistory(path, QueryOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(HaveLen(1))
		Expect(history[0].DispatchKey).To(Equal("go:test"))
		Expect(history[0].DurationMS).To(Equal(int64(275)))
		Expect(history[0].Passthrough).To(BeTrue())
	})

	It("preserves filter provenance in history rows", func() {
		path := filepath.Join(tempDir, "metrics.db")

		Expect(Append(path, RunMetric{
			Tool:             "py",
			Command:          "py -m pytest",
			Dispatch:         "python|pytest",
			RawBytes:         18,
			KeptBytes:        6,
			FilterSourceKind: "project",
			FilterPath:       "/repo/.cmdshape/filters/python.yaml",
			FilterHash:       "abc123",
		})).To(Succeed())

		history, err := QueryHistory(path, QueryOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(HaveLen(1))
		Expect(history[0].Tool).To(Equal("py"))
		Expect(history[0].Filter).To(Equal("python"))
		Expect(history[0].Case).To(Equal("pytest"))
		Expect(history[0].FilterSourceKind).To(Equal("project"))
		Expect(history[0].FilterPath).To(Equal("/repo/.cmdshape/filters/python.yaml"))
		Expect(history[0].FilterHash).To(Equal("abc123"))
	})

	Context("when storing tool names", func() {
		var path string

		BeforeEach(func() {
			path = filepath.Join(tempDir, "metrics.db")
		})

		It("preserves an explicit tool name", func() {
			Expect(Append(path, RunMetric{
				Tool:        "git",
				Command:     "git ls-files --stage",
				RawBytes:    10,
				KeptBytes:   10,
				Passthrough: true,
			})).To(Succeed())

			history, err := QueryHistory(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(HaveLen(1))
			Expect(history[0].Tool).To(Equal("git"))
			Expect(history[0].Passthrough).To(BeTrue())
		})

		It("normalizes a blank tool name to unknown", func() {
			Expect(Append(path, RunMetric{
				Command:     "echo a && echo b",
				RawBytes:    4,
				KeptBytes:   4,
				Passthrough: true,
			})).To(Succeed())

			history, err := QueryHistory(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(HaveLen(1))
			Expect(history[0].Tool).To(Equal("unknown"))
		})
	})

	Describe("updating local project gitignore for metrics db", func() {
		It("ensures the nested cmdshape gitignore and leaves the parent gitignore unchanged", func() {
			project := initGitProjectForMetrics(tempDir, "node_modules/\n.cmdshape\n")
			withMetricsWorkingDir(project)
			path := filepath.Join(project, ".cmdshape", gainDBFileName)

			Expect(Append(path, RunMetric{Tool: "go", Command: metricsGoTestCommand, RawBytes: 16, KeptBytes: 8})).To(Succeed())
			Expect(os.WriteFile(filepath.Join(project, ".cmdshape", gitignoreFileName), []byte("user-edit\n"), 0o644)).To(Succeed())
			Expect(Append(path, RunMetric{Tool: "go", Command: metricsGoTestCommand, RawBytes: 16, KeptBytes: 8})).To(Succeed())

			parent, err := os.ReadFile(filepath.Join(project, gitignoreFileName))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(parent)).To(Equal("node_modules/\n.cmdshape\n"))
			nested, err := os.ReadFile(filepath.Join(project, ".cmdshape", gitignoreFileName))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(nested)).To(Equal("gain.db\n.gitignore\n"))
		})

		It("overwrites stale nested cmdshape gitignore contents", func() {
			project := initGitProjectForMetrics(tempDir, "node_modules/\n")
			withMetricsWorkingDir(project)
			cmdshapeDir := filepath.Join(project, ".cmdshape")
			Expect(os.MkdirAll(cmdshapeDir, 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(cmdshapeDir, gitignoreFileName), []byte("custom\n"), 0o644)).To(Succeed())

			Expect(Append(filepath.Join(cmdshapeDir, gainDBFileName), RunMetric{Tool: "go", Command: metricsGoTestCommand, RawBytes: 16, KeptBytes: 8})).To(Succeed())

			nested, err := os.ReadFile(filepath.Join(cmdshapeDir, gitignoreFileName))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(nested)).To(Equal("gain.db\n.gitignore\n"))
		})

		It("skips nested ignore management when current directory is not the git root", func() {
			project := initGitProjectForMetrics(tempDir, "node_modules/\n")
			subdir := filepath.Join(project, "subdir")
			Expect(os.MkdirAll(subdir, 0o755)).To(Succeed())
			withMetricsWorkingDir(subdir)
			path := filepath.Join(subdir, ".cmdshape", gainDBFileName)

			Expect(Append(path, RunMetric{Tool: "go", Command: metricsGoTestCommand, RawBytes: 16, KeptBytes: 8})).To(Succeed())

			_, err := os.Stat(filepath.Join(subdir, ".cmdshape", gitignoreFileName))
			Expect(err).To(MatchError(os.ErrNotExist))
			parent, err := os.ReadFile(filepath.Join(project, gitignoreFileName))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(parent)).To(Equal("node_modules/\n"))
		})

		It("skips nested ignore management outside git repositories", func() {
			project := tempDir
			withMetricsWorkingDir(project)
			path := filepath.Join(project, ".cmdshape", gainDBFileName)

			Expect(Append(path, RunMetric{Tool: "go", Command: metricsGoTestCommand, RawBytes: 16, KeptBytes: 8})).To(Succeed())

			_, err := os.Stat(filepath.Join(project, ".cmdshape", gitignoreFileName))
			Expect(err).To(MatchError(os.ErrNotExist))
		})

		It("skips non-cmdshape metrics paths", func() {
			project := initGitProjectForMetrics(tempDir, "node_modules/\n")
			withMetricsWorkingDir(project)

			Expect(Append(filepath.Join(project, "metrics", gainDBFileName), RunMetric{Tool: "go", Command: metricsGoTestCommand, RawBytes: 16, KeptBytes: 8})).To(Succeed())

			_, err := os.Stat(filepath.Join(project, ".cmdshape", gitignoreFileName))
			Expect(err).To(MatchError(os.ErrNotExist))
		})

		It("leaves filters visible to git while ignoring generated metrics state", func() {
			if _, err := exec.LookPath("git"); err != nil {
				Skip("git unavailable: " + err.Error())
			}
			project := tempDir
			cmd := exec.Command("git", "init")
			cmd.Dir = project
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), string(out))
			withMetricsWorkingDir(project)
			filtersDir := filepath.Join(project, ".cmdshape", "filters")
			Expect(os.MkdirAll(filtersDir, 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(filtersDir, "local.yaml"), []byte("version: 1\n"), 0o644)).To(Succeed())

			Expect(Append(filepath.Join(project, ".cmdshape", gainDBFileName), RunMetric{Tool: "go", Command: metricsGoTestCommand, RawBytes: 16, KeptBytes: 8})).To(Succeed())

			ignored := metricsGitCheckIgnore(project, ".cmdshape/gain.db", ".cmdshape/.gitignore")
			Expect(ignored).To(ContainElement(".cmdshape/gain.db"))
			Expect(ignored).To(ContainElement(".cmdshape/.gitignore"))
			Expect(metricsGitCheckIgnore(project, ".cmdshape/filters/local.yaml")).To(BeEmpty())
		})

		It("propagates nested ignore symlink errors", func() {
			project := initGitProjectForMetrics(tempDir, "node_modules/\n")
			withMetricsWorkingDir(project)
			outside := filepath.Join(GinkgoT().TempDir(), "outside-gitignore")
			cmdshapeDir := filepath.Join(project, ".cmdshape")
			Expect(os.MkdirAll(cmdshapeDir, 0o755)).To(Succeed())
			Expect(os.WriteFile(outside, []byte("keep\n"), 0o644)).To(Succeed())
			if err := os.Symlink(outside, filepath.Join(cmdshapeDir, gitignoreFileName)); err != nil {
				Skip("symlink creation unavailable: " + err.Error())
			}

			err := Append(filepath.Join(cmdshapeDir, gainDBFileName), RunMetric{Tool: "go", Command: metricsGoTestCommand, RawBytes: 16, KeptBytes: 8})

			Expect(err).To(HaveOccurred())
			body, readErr := os.ReadFile(outside)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal("keep\n"))
		})
	})

	DescribeTable("encoding helper bounds",
		func(value int, expected uint32) {
			dst := make([]byte, 4)
			putLengthU32(dst, value)
			Expect(getU32(dst)).To(Equal(expected))
		},
		Entry("clamps negative lengths to zero", -1, uint32(0)),
		Entry("preserves zero lengths", 0, uint32(0)),
		Entry("preserves in-range lengths", 42, uint32(42)),
		Entry("preserves the max uint32 boundary exactly", int(math.MaxUint32), uint32(math.MaxUint32)),
		Entry("clamps oversized lengths to max uint32", int(math.MaxUint32)+1, uint32(math.MaxUint32)),
	)

	DescribeTable("non-negative integer helpers",
		func(value int64, expected uint64) {
			dst := make([]byte, 8)
			putNonNegativeInt64AsU64(dst, value)
			Expect(getU64(dst)).To(Equal(expected))
		},
		Entry("clamps negative values to zero", int64(-1), uint64(0)),
		Entry("preserves zero values", int64(0), uint64(0)),
		Entry("preserves positive values", int64(17), uint64(17)),
	)

	DescribeTable("token estimation rounds up every four bytes",
		func(bytes int64, expected int64) {
			Expect(tokensFromBytes(bytes)).To(Equal(expected))
		},
		Entry("clamps negative bytes to zero", int64(-5), int64(0)),
		Entry("treats zero bytes as zero tokens", int64(0), int64(0)),
		Entry("rounds one byte up to one token", int64(1), int64(1)),
		Entry("keeps four bytes at one token", int64(4), int64(1)),
		Entry("rounds five bytes up to two tokens", int64(5), int64(2)),
	)

	DescribeTable("bounded integer decoders",
		func(src []byte, decode func([]byte) int64, expected int64) {
			Expect(decode(src)).To(Equal(expected))
		},
		Entry("clamps oversized int64 values", []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, getBoundedInt64FromU64, int64(math.MaxInt64)),
		Entry("preserves the max int64 boundary exactly", encodedU64(uint64(math.MaxInt64)), getBoundedInt64FromU64, int64(math.MaxInt64)),
		Entry("preserves smaller int64 values", []byte{0, 0, 0, 0, 0, 0, 0, 9}, getBoundedInt64FromU64, int64(9)),
	)

	DescribeTable("bounded signed int decoders",
		func(src []byte, expected int) {
			Expect(getBoundedSignedIntFromU64(src)).To(Equal(expected))
		},
		Entry("preserves the max int boundary exactly", encodedU64(uint64(int64(math.MaxInt))), math.MaxInt),
		Entry("preserves the min int boundary exactly", encodedU64(uint64(1)<<63), math.MinInt),
		Entry("preserves a positive exit code", []byte{0, 0, 0, 0, 0, 0, 0, 7}, 7),
		Entry("preserves a negative exit code", []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfd}, -3),
	)

	DescribeTable("bounded uint32 decoders",
		func(src []byte, expected int) {
			Expect(getBoundedIntFromU32(src)).To(Equal(expected))
		},
		Entry("preserves a small uint32", []byte{0, 0, 0, 9}, 9),
		Entry("preserves max uint32 on 64-bit hosts", []byte{0xff, 0xff, 0xff, 0xff}, int(^uint32(0))),
	)

	DescribeTable("busy error detection",
		func(err error, expected bool) {
			Expect(IsTimeoutOrBusy(err)).To(Equal(expected))
		},
		Entry("ignores nil", nil, false),
		Entry("matches timeout text", errors.New("write timeout"), true),
		Entry("matches busy text", errors.New("resource busy"), true),
		Entry("matches sqlite style lock text", errors.New("database is locked"), true),
		Entry("ignores unrelated errors", errors.New("permission denied"), false),
	)

	Context("mutation hardening helpers", func() {
		It("groups summary rows by command and sorts ties alphabetically", func() {
			path := filepath.Join(tempDir, "metrics.db")
			appendRunMetrics(path,
				RunMetric{Timestamp: time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC), Tool: "go", Command: "beta", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 10, 1, 0, 0, time.UTC), Tool: "go", Command: "alpha", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 10, 2, 0, 0, time.UTC), Tool: "go", Command: "beta", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 10, 3, 0, 0, time.UTC), Tool: "go", Command: "alpha", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 10, 4, 0, 0, time.UTC), Tool: "go", Command: "gamma", RawBytes: 4, KeptBytes: 4},
			)

			rows, err := QuerySummaryRows(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(Equal([]SummaryRow{
				{
					Command:               "alpha",
					Commands:              2,
					RawBytes:              16,
					KeptBytes:             8,
					DroppedBytes:          8,
					DropRatio:             0.5,
					EstimatedInputTokens:  4,
					EstimatedOutputTokens: 2,
					EstimatedSavedTokens:  2,
					EstimatedSavingsPct:   50,
				},
				{
					Command:               "beta",
					Commands:              2,
					RawBytes:              16,
					KeptBytes:             8,
					DroppedBytes:          8,
					DropRatio:             0.5,
					EstimatedInputTokens:  4,
					EstimatedOutputTokens: 2,
					EstimatedSavedTokens:  2,
					EstimatedSavingsPct:   50,
				},
				{
					Command:               "gamma",
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
		})

		It("groups summary rows by tool, normalizes unknown tools, and sorts ties alphabetically", func() {
			path := filepath.Join(tempDir, "metrics.db")
			appendRunMetrics(path,
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 0, 0, 0, time.UTC), Tool: "", Command: "echo alpha", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 1, 0, 0, time.UTC), Tool: "git", Command: "git status", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 2, 0, 0, time.UTC), Tool: "", Command: "echo beta", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 3, 0, 0, time.UTC), Tool: "git", Command: "git diff", RawBytes: 8, KeptBytes: 4},
			)

			rows, err := QuerySummaryRowsByTool(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(Equal([]SummaryToolRow{
				{
					Tool:                  "git",
					Commands:              2,
					RawBytes:              16,
					KeptBytes:             8,
					DroppedBytes:          8,
					DropRatio:             0.5,
					EstimatedInputTokens:  4,
					EstimatedOutputTokens: 2,
					EstimatedSavedTokens:  2,
					EstimatedSavingsPct:   50,
				},
				{
					Tool:                  "unknown",
					Commands:              2,
					RawBytes:              16,
					KeptBytes:             8,
					DroppedBytes:          8,
					DropRatio:             0.5,
					EstimatedInputTokens:  4,
					EstimatedOutputTokens: 2,
					EstimatedSavedTokens:  2,
					EstimatedSavingsPct:   50,
				},
			}))
		})

		It("sorts summary rows by command count before alphabetical tiebreaks", func() {
			path := filepath.Join(tempDir, "metrics.db")
			appendRunMetrics(path,
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 10, 0, 0, time.UTC), Tool: "go", Command: "zeta", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 11, 0, 0, time.UTC), Tool: "go", Command: "beta", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 12, 0, 0, time.UTC), Tool: "go", Command: "alpha", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 13, 0, 0, time.UTC), Tool: "go", Command: "zeta", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 14, 0, 0, time.UTC), Tool: "go", Command: "beta", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 15, 0, 0, time.UTC), Tool: "go", Command: "alpha", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 16, 0, 0, time.UTC), Tool: "go", Command: "zeta", RawBytes: 8, KeptBytes: 4},
			)

			rows, err := QuerySummaryRows(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(3))
			Expect(rows[0].Command).To(Equal("zeta"))
			Expect(rows[0].Commands).To(Equal(int64(3)))
			Expect(rows[1].Command).To(Equal("alpha"))
			Expect(rows[1].Commands).To(Equal(int64(2)))
			Expect(rows[2].Command).To(Equal("beta"))
			Expect(rows[2].Commands).To(Equal(int64(2)))
		})

		It("sorts summary rows with higher counts first before applying alphabetical tiebreaks", func() {
			path := filepath.Join(tempDir, "metrics.db")
			appendRunMetrics(path,
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 10, 0, 0, time.UTC), Tool: "go", Command: "zeta", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 11, 0, 0, time.UTC), Tool: "go", Command: "zeta", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 12, 0, 0, time.UTC), Tool: "go", Command: "zeta", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 13, 0, 0, time.UTC), Tool: "go", Command: "beta", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 14, 0, 0, time.UTC), Tool: "go", Command: "beta", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 15, 0, 0, time.UTC), Tool: "go", Command: "alpha", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 16, 0, 0, time.UTC), Tool: "go", Command: "alpha", RawBytes: 8, KeptBytes: 4},
			)

			rows, err := QuerySummaryRows(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(3))
			Expect(rows[0].Command).To(Equal("zeta"))
			Expect(rows[1].Command).To(Equal("alpha"))
			Expect(rows[2].Command).To(Equal("beta"))
			Expect(rows[0].Commands).To(Equal(int64(3)))
			Expect(rows[1].Commands).To(Equal(int64(2)))
			Expect(rows[2].Commands).To(Equal(int64(2)))
		})

		It("sorts tool summary rows with higher counts first before applying alphabetical tiebreaks", func() {
			path := filepath.Join(tempDir, "metrics.db")
			appendRunMetrics(path,
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 20, 0, 0, time.UTC), Tool: "zeta", Command: "one", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 21, 0, 0, time.UTC), Tool: "zeta", Command: "two", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 22, 0, 0, time.UTC), Tool: "zeta", Command: "three", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 23, 0, 0, time.UTC), Tool: "beta", Command: "four", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 24, 0, 0, time.UTC), Tool: "beta", Command: "five", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 25, 0, 0, time.UTC), Tool: "alpha", Command: "six", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 11, 26, 0, 0, time.UTC), Tool: "alpha", Command: "seven", RawBytes: 8, KeptBytes: 4},
			)

			rows, err := QuerySummaryRowsByTool(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(3))
			Expect(rows[0].Tool).To(Equal("zeta"))
			Expect(rows[1].Tool).To(Equal("alpha"))
			Expect(rows[2].Tool).To(Equal("beta"))
			Expect(rows[0].Commands).To(Equal(int64(3)))
			Expect(rows[1].Commands).To(Equal(int64(2)))
			Expect(rows[2].Commands).To(Equal(int64(2)))
		})

		It("groups performance rows by invoked tool, filter case, and provenance", func() {
			path := filepath.Join(tempDir, "metrics.db")
			appendRunMetrics(path,
				RunMetric{
					Timestamp:        time.Date(2026, 3, 25, 11, 20, 0, 0, time.UTC),
					Tool:             "py",
					Command:          "py -m pytest",
					Dispatch:         "python|pytest",
					RawBytes:         16,
					KeptBytes:        4,
					DurationMS:       20,
					FilterSourceKind: "project",
					FilterPath:       "/repo/.cmdshape/filters/python.yaml",
					FilterHash:       "hash-a",
				},
				RunMetric{
					Timestamp:        time.Date(2026, 3, 25, 11, 21, 0, 0, time.UTC),
					Tool:             "py",
					Command:          "py -m pytest",
					Dispatch:         "python|pytest",
					RawBytes:         8,
					KeptBytes:        8,
					ExitCode:         1,
					DurationMS:       40,
					Passthrough:      true,
					FilterSourceKind: "project",
					FilterPath:       "/repo/.cmdshape/filters/python.yaml",
					FilterHash:       "hash-a",
				},
				RunMetric{
					Timestamp:        time.Date(2026, 3, 25, 11, 22, 0, 0, time.UTC),
					Tool:             "python",
					Command:          "python script.py",
					Dispatch:         "python|default",
					RawBytes:         12,
					KeptBytes:        6,
					FilterSourceKind: "home",
					FilterPath:       "/home/user/.config/cmdshape/filters/python.yaml",
					FilterHash:       "hash-b",
				},
			)

			rows, err := QueryPerformanceRows(path, QueryOptions{})

			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(2))
			Expect(rows[0].Tool).To(Equal("py"))
			Expect(rows[0].Filter).To(Equal("python"))
			Expect(rows[0].Case).To(Equal("pytest"))
			Expect(rows[0].DispatchKey).To(Equal("python|pytest"))
			Expect(rows[0].FilterSourceKind).To(Equal("project"))
			Expect(rows[0].FilterPath).To(Equal("/repo/.cmdshape/filters/python.yaml"))
			Expect(rows[0].FilterHash).To(Equal("hash-a"))
			Expect(rows[0].Commands).To(Equal(int64(2)))
			Expect(rows[0].PassthroughCommands).To(Equal(int64(1)))
			Expect(rows[0].PassthroughRate).To(Equal(0.5))
			Expect(rows[0].FailedCommands).To(Equal(int64(1)))
			Expect(rows[0].FailedRate).To(Equal(0.5))
			Expect(rows[0].AvgDurationMS).To(Equal(float64(30)))
			Expect(rows[0].RawBytes).To(Equal(int64(24)))
			Expect(rows[0].KeptBytes).To(Equal(int64(12)))
			Expect(rows[0].EstimatedSavingsPct).To(Equal(float64(50)))

			Expect(rows[1].Tool).To(Equal("python"))
			Expect(rows[1].Filter).To(Equal("python"))
			Expect(rows[1].Case).To(Equal("default"))
			Expect(rows[1].Commands).To(Equal(int64(1)))
		})

		It("aggregates registry build timing rows and sources", func() {
			path := filepath.Join(tempDir, "metrics.db")
			appendRunMetrics(path,
				RunMetric{
					Timestamp:             time.Date(2026, 3, 25, 11, 30, 0, 0, time.UTC),
					Tool:                  "go",
					Command:               "go test ./...",
					RawBytes:              8,
					KeptBytes:             4,
					RegistryBuildRecorded: true,
					RegistryBuildMS:       10,
					RegistrySources: []RegistrySourceBuildMetric{
						{SourceKind: "project", SourceDir: "/repo/.cmdshape/filters", Definitions: 2, Compiled: 2, DurationMS: 7},
						{SourceKind: "home", SourceDir: "/home/user/.config/cmdshape/filters", Definitions: 1, Compiled: 1, DurationMS: 3},
					},
				},
				RunMetric{
					Timestamp:             time.Date(2026, 3, 25, 11, 31, 0, 0, time.UTC),
					Tool:                  "go",
					Command:               "go test ./...",
					RawBytes:              8,
					KeptBytes:             4,
					RegistryBuildRecorded: true,
					RegistryBuildMS:       30,
					RegistrySources: []RegistrySourceBuildMetric{
						{SourceKind: "project", SourceDir: "/repo/.cmdshape/filters", Definitions: 3, Compiled: 2, DurationMS: 25, Error: "boom"},
					},
				},
				RunMetric{
					Timestamp:       time.Date(2026, 3, 25, 11, 32, 0, 0, time.UTC),
					Tool:            "go",
					Command:         "legacy",
					RawBytes:        8,
					KeptBytes:       4,
					RegistryBuildMS: 99,
				},
			)

			summary, rows, err := QueryRegistryBuild(path, QueryOptions{})

			Expect(err).NotTo(HaveOccurred())
			Expect(summary.Builds).To(Equal(int64(2)))
			Expect(summary.AvgDurationMS).To(Equal(float64(20)))
			Expect(summary.P95DurationMS).To(Equal(int64(30)))
			Expect(summary.MaxDurationMS).To(Equal(int64(30)))
			Expect(rows).To(HaveLen(2))
			Expect(rows[0].SourceKind).To(Equal("project"))
			Expect(rows[0].SourceDir).To(Equal("/repo/.cmdshape/filters"))
			Expect(rows[0].Builds).To(Equal(int64(2)))
			Expect(rows[0].Errors).To(Equal(int64(1)))
			Expect(rows[0].Definitions).To(Equal(int64(5)))
			Expect(rows[0].Compiled).To(Equal(int64(4)))
			Expect(rows[0].AvgDurationMS).To(Equal(float64(16)))
			Expect(rows[0].MaxDurationMS).To(Equal(int64(25)))
			Expect(rows[1].SourceKind).To(Equal("home"))
			Expect(rows[1].Builds).To(Equal(int64(1)))
		})

		It("ranks missed opportunities by count, breaks ties alphabetically, and applies the default limit", func() {
			path := filepath.Join(tempDir, "metrics.db")
			appendRunMetrics(path,
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC), Tool: "go", Command: "beta", Passthrough: true},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 1, 0, 0, time.UTC), Tool: "go", Command: "alpha", Passthrough: true},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 2, 0, 0, time.UTC), Tool: "go", Command: "beta", Passthrough: true},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 3, 0, 0, time.UTC), Tool: "go", Command: "alpha", Passthrough: true},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 4, 0, 0, time.UTC), Tool: "go", Command: "delta", Passthrough: true},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 5, 0, 0, time.UTC), Tool: "go", Command: "epsilon", Passthrough: true},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 6, 0, 0, time.UTC), Tool: "go", Command: "eta", Passthrough: true},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 7, 0, 0, time.UTC), Tool: "go", Command: "gamma", Passthrough: true},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 8, 0, 0, time.UTC), Tool: "go", Command: "zeta", Passthrough: true},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 9, 0, 0, time.UTC), Tool: "go", Command: "ignored", Passthrough: false},
			)

			rows, err := QueryMissedOpportunities(path, QueryOptions{}, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(Equal([]MissedOpportunity{
				{Command: "alpha", Count: 2},
				{Command: "beta", Count: 2},
				{Command: "delta", Count: 1},
				{Command: "epsilon", Count: 1},
				{Command: "eta", Count: 1},
			}))
		})

		It("keeps all missed opportunities when the explicit limit matches the result count", func() {
			path := filepath.Join(tempDir, "metrics.db")
			appendRunMetrics(path,
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC), Tool: "go", Command: "beta", Passthrough: true},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 1, 0, 0, time.UTC), Tool: "go", Command: "alpha", Passthrough: true},
			)

			rows, err := QueryMissedOpportunities(path, QueryOptions{}, 2)
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(Equal([]MissedOpportunity{
				{Command: "alpha", Count: 1},
				{Command: "beta", Count: 1},
			}))
		})

		It("treats negative limits as requests for the default missed-opportunity cap", func() {
			path := filepath.Join(tempDir, "metrics.db")
			appendRunMetrics(path,
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC), Tool: "go", Command: "alpha", Passthrough: true},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 1, 0, 0, time.UTC), Tool: "go", Command: "beta", Passthrough: true},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 2, 0, 0, time.UTC), Tool: "go", Command: "gamma", Passthrough: true},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 3, 0, 0, time.UTC), Tool: "go", Command: "delta", Passthrough: true},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 4, 0, 0, time.UTC), Tool: "go", Command: "epsilon", Passthrough: true},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 5, 0, 0, time.UTC), Tool: "go", Command: "zeta", Passthrough: true},
			)

			rows, err := QueryMissedOpportunities(path, QueryOptions{}, -1)
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(defaultMissedTopLimit))
			Expect(rows).NotTo(ContainElement(MissedOpportunity{Command: "zeta", Count: 1}))
		})

		DescribeTable("applies explicit missed-opportunity limits only when needed",
			func(limit int, expected []MissedOpportunity) {
				path := filepath.Join(tempDir, "metrics.db")
				appendRunMetrics(path,
					RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC), Tool: "go", Command: "gamma", Passthrough: true},
					RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 1, 0, 0, time.UTC), Tool: "go", Command: "beta", Passthrough: true},
					RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 2, 0, 0, time.UTC), Tool: "go", Command: "alpha", Passthrough: true},
				)

				rows, err := QueryMissedOpportunities(path, QueryOptions{}, limit)
				Expect(err).NotTo(HaveOccurred())
				Expect(rows).To(Equal(expected))
			},
			Entry("keeps all rows when the limit matches the result size", 3, []MissedOpportunity{
				{Command: "alpha", Count: 1},
				{Command: "beta", Count: 1},
				{Command: "gamma", Count: 1},
			}),
			Entry("truncates rows when the limit is smaller than the result size", 2, []MissedOpportunity{
				{Command: "alpha", Count: 1},
				{Command: "beta", Count: 1},
			}),
		)

		It("keeps all missed opportunities when the explicit limit matches the result count", func() {
			path := filepath.Join(tempDir, "metrics.db")
			appendRunMetrics(path,
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 10, 0, 0, time.UTC), Tool: "go", Command: "beta", Passthrough: true},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 12, 11, 0, 0, time.UTC), Tool: "go", Command: "alpha", Passthrough: true},
			)

			rows, err := QueryMissedOpportunities(path, QueryOptions{}, 2)
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(Equal([]MissedOpportunity{
				{Command: "alpha", Count: 1},
				{Command: "beta", Count: 1},
			}))
		})

		It("groups period rows by bucket and sorts the newest bucket first", func() {
			path := filepath.Join(tempDir, "metrics.db")
			appendRunMetrics(path,
				RunMetric{Timestamp: time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC), Tool: "go", Command: "day-two-a", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 2, 9, 1, 0, 0, time.UTC), Tool: "go", Command: "day-two-b", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC), Tool: "go", Command: "day-one-a", RawBytes: 4, KeptBytes: 0},
				RunMetric{Timestamp: time.Date(2026, 3, 1, 9, 1, 0, 0, time.UTC), Tool: "go", Command: "day-one-b", RawBytes: 4, KeptBytes: 0},
			)

			rows, err := QueryPeriod(path, QueryOptions{Period: "day"})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(Equal([]PeriodRow{
				{
					Bucket:                "2026-03-02",
					BucketStart:           "2026-03-02",
					BucketEnd:             "2026-03-02",
					Commands:              2,
					RawBytes:              16,
					KeptBytes:             8,
					DroppedBytes:          8,
					DropRatio:             0.5,
					EstimatedInputTokens:  4,
					EstimatedOutputTokens: 2,
					EstimatedSavedTokens:  2,
					EstimatedSavingsPct:   50,
				},
				{
					Bucket:                "2026-03-01",
					BucketStart:           "2026-03-01",
					BucketEnd:             "2026-03-01",
					Commands:              2,
					RawBytes:              8,
					KeptBytes:             0,
					DroppedBytes:          8,
					DropRatio:             1,
					EstimatedInputTokens:  2,
					EstimatedOutputTokens: 0,
					EstimatedSavedTokens:  2,
					EstimatedSavingsPct:   100,
				},
			}))
		})

		It("sorts tool summaries by command count before alphabetical tiebreaks", func() {
			path := filepath.Join(tempDir, "metrics.db")
			appendRunMetrics(path,
				RunMetric{Timestamp: time.Date(2026, 3, 25, 13, 0, 0, 0, time.UTC), Tool: "zeta", Command: "zeta one", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 13, 1, 0, 0, time.UTC), Tool: "beta", Command: "beta one", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 13, 2, 0, 0, time.UTC), Tool: "alpha", Command: "alpha one", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 13, 3, 0, 0, time.UTC), Tool: "zeta", Command: "zeta two", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 13, 4, 0, 0, time.UTC), Tool: "beta", Command: "beta two", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 13, 5, 0, 0, time.UTC), Tool: "alpha", Command: "alpha two", RawBytes: 8, KeptBytes: 4},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 13, 6, 0, 0, time.UTC), Tool: "zeta", Command: "zeta three", RawBytes: 8, KeptBytes: 4},
			)

			rows, err := QuerySummaryRowsByTool(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(3))
			Expect(rows[0].Tool).To(Equal("zeta"))
			Expect(rows[0].Commands).To(Equal(int64(3)))
			Expect(rows[1].Tool).To(Equal("alpha"))
			Expect(rows[1].Commands).To(Equal(int64(2)))
			Expect(rows[2].Tool).To(Equal("beta"))
			Expect(rows[2].Commands).To(Equal(int64(2)))
		})

		It("returns invalid period errors from period queries", func() {
			path := filepath.Join(tempDir, "metrics.db")
			appendRunMetrics(path, RunMetric{
				Timestamp: time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC),
				Tool:      "go",
				Command:   metricsGoTestCommand,
				RawBytes:  8,
				KeptBytes: 4,
			})

			_, err := QueryPeriod(path, QueryOptions{Period: "quarter"})
			Expect(err).To(MatchError(`invalid period "quarter"`))
		})

		It("returns filtered history in reverse chronological order", func() {
			path := filepath.Join(tempDir, "metrics.db")
			appendRunMetrics(path,
				RunMetric{Timestamp: time.Date(2026, 3, 25, 9, 0, 0, 0, time.UTC), Tool: "go", Command: "old success", RawBytes: 8, KeptBytes: 4, ExitCode: 0},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 9, 1, 0, 0, time.UTC), Tool: "go", Command: "new failure", RawBytes: 8, KeptBytes: 4, ExitCode: 2},
				RunMetric{Timestamp: time.Date(2026, 3, 25, 9, 2, 0, 0, time.UTC), Tool: "git", Command: "ignored tool", RawBytes: 8, KeptBytes: 4, ExitCode: 3},
			)

			history, err := QueryHistory(path, QueryOptions{Tool: "go"})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(HaveLen(2))
			Expect(history[0].Command).To(Equal("new failure"))
			Expect(history[1].Command).To(Equal("old success"))
		})

		DescribeTable("derives history rows exactly",
			func(rec runRecord, expectedFailed bool, expected derivedExpectation) {
				got := historyRowFromRecord(rec)

				Expect(got.Timestamp).To(Equal(time.Unix(rec.TimestampUnix, 0).UTC()))
				Expect(got.Command).To(Equal(rec.Command))
				Expect(got.Tool).To(Equal(rec.Tool))
				Expect(got.DispatchKey).To(Equal(rec.Dispatch))
				Expect(got.ExitCode).To(Equal(rec.ExitCode))
				Expect(got.Failed).To(Equal(expectedFailed))
				Expect(got.Passthrough).To(Equal(rec.Passthrough))
				Expect(got.DurationMS).To(Equal(rec.DurationMS))
				Expect(got.DroppedBytes).To(Equal(expected.dropped))
				Expect(got.DropRatio).To(Equal(expected.ratio))
				Expect(got.EstimatedInputTokens).To(Equal(expected.inputTokens))
				Expect(got.EstimatedOutputTokens).To(Equal(expected.outputTokens))
				Expect(got.EstimatedSavedTokens).To(Equal(expected.savedTokens))
				Expect(got.EstimatedSavingsPct).To(Equal(expected.savingsPct))
			},
			Entry("handles zero raw bytes without ratios or token estimates",
				runRecord{TimestampUnix: 1_700_000_000, Command: "echo", Tool: "go", Dispatch: "go:test", RawBytes: 0, KeptBytes: 0, ExitCode: 0},
				false,
				derivedExpectation{},
			),
			Entry("derives dropped bytes, ratios, and token savings from compressed output",
				runRecord{TimestampUnix: 1_700_000_010, Command: metricsGoTestCommand, Tool: "go", Dispatch: "go:test", RawBytes: 8, KeptBytes: 4, ExitCode: 2, DurationMS: 25, Passthrough: true},
				true,
				derivedExpectation{dropped: 4, ratio: 0.5, inputTokens: 2, outputTokens: 1, savedTokens: 1, savingsPct: 50},
			),
			Entry("clamps derived savings when kept bytes exceed raw bytes",
				runRecord{TimestampUnix: 1_700_000_020, Command: "synthetic-summary", Tool: "sh", Dispatch: "sh|summary", RawBytes: 8, KeptBytes: 42, ExitCode: 0},
				false,
				derivedExpectation{dropped: 0, ratio: 0, inputTokens: 2, outputTokens: 2, savedTokens: 0, savingsPct: 0},
			),
		)

		It("fills derived summary fields consistently across summary row types", func() {
			row := SummaryRow{RawBytes: 8, KeptBytes: 4}
			fillSummaryDerived(&row)
			Expect(row.DroppedBytes).To(Equal(int64(4)))
			Expect(row.DropRatio).To(Equal(0.5))
			Expect(row.EstimatedInputTokens).To(Equal(int64(2)))
			Expect(row.EstimatedOutputTokens).To(Equal(int64(1)))
			Expect(row.EstimatedSavedTokens).To(Equal(int64(1)))
			Expect(row.EstimatedSavingsPct).To(Equal(50.0))

			tool := SummaryToolRow{RawBytes: 8, KeptBytes: 4}
			fillSummaryToolDerived(&tool)
			Expect(tool.DroppedBytes).To(Equal(int64(4)))
			Expect(tool.DropRatio).To(Equal(0.5))
			Expect(tool.EstimatedInputTokens).To(Equal(int64(2)))
			Expect(tool.EstimatedOutputTokens).To(Equal(int64(1)))
			Expect(tool.EstimatedSavedTokens).To(Equal(int64(1)))
			Expect(tool.EstimatedSavingsPct).To(Equal(50.0))

			total := SummaryTotal{RawBytes: 8, KeptBytes: 4}
			fillTotalDerived(&total)
			Expect(total.DroppedBytes).To(Equal(int64(4)))
			Expect(total.DropRatio).To(Equal(0.5))
			Expect(total.EstimatedInputTokens).To(Equal(int64(2)))
			Expect(total.EstimatedOutputTokens).To(Equal(int64(1)))
			Expect(total.EstimatedSavedTokens).To(Equal(int64(1)))
			Expect(total.EstimatedSavingsPct).To(Equal(50.0))
		})

		It("leaves summary ratios and token savings at zero when raw bytes are zero", func() {
			row := SummaryRow{RawBytes: 0, KeptBytes: 0}
			fillSummaryDerived(&row)
			Expect(row).To(Equal(SummaryRow{}))

			tool := SummaryToolRow{RawBytes: 0, KeptBytes: 0}
			fillSummaryToolDerived(&tool)
			Expect(tool).To(Equal(SummaryToolRow{}))

			total := SummaryTotal{RawBytes: 0, KeptBytes: 0}
			fillTotalDerived(&total)
			Expect(total).To(Equal(SummaryTotal{}))
		})

		DescribeTable("derives period rows exactly",
			func(acc *periodAcc, expected derivedExpectation) {
				got := periodRowFromAcc(acc)

				Expect(got.Bucket).To(Equal(acc.bucket))
				Expect(got.BucketStart).To(Equal(acc.start))
				Expect(got.BucketEnd).To(Equal(acc.end))
				Expect(got.Commands).To(Equal(acc.count))
				Expect(got.DroppedBytes).To(Equal(expected.dropped))
				Expect(got.DropRatio).To(Equal(expected.ratio))
				Expect(got.EstimatedInputTokens).To(Equal(expected.inputTokens))
				Expect(got.EstimatedOutputTokens).To(Equal(expected.outputTokens))
				Expect(got.EstimatedSavedTokens).To(Equal(expected.savedTokens))
				Expect(got.EstimatedSavingsPct).To(Equal(expected.savingsPct))
			},
			Entry("handles empty raw bytes without derived ratios",
				&periodAcc{bucket: "2026-03-26", start: "2026-03-26", end: "2026-03-26", count: 1},
				derivedExpectation{},
			),
			Entry("derives dropped bytes, ratios, and token savings for compressed rows",
				&periodAcc{bucket: "2026-03-26", start: "2026-03-26", end: "2026-03-26", count: 2, raw: 8, kept: 4},
				derivedExpectation{dropped: 4, ratio: 0.5, inputTokens: 2, outputTokens: 1, savedTokens: 1, savingsPct: 50},
			),
		)

		It("rejects malformed encoded records", func() {
			shortRecord := make([]byte, 8+4+4+4+8+8+8+8)

			truncatedCommand := make([]byte, 12)
			putU32(truncatedCommand[8:12], 5)

			truncatedTool := make([]byte, 19)
			putU32(truncatedTool[8:12], 3)
			copy(truncatedTool[12:15], []byte("cmd"))
			putU32(truncatedTool[15:19], 2)

			truncatedDispatch := make([]byte, 25)
			putU32(truncatedDispatch[8:12], 3)
			copy(truncatedDispatch[12:15], []byte("cmd"))
			putU32(truncatedDispatch[15:19], 2)
			copy(truncatedDispatch[19:21], []byte("go"))
			putU32(truncatedDispatch[21:25], 4)

			truncatedTail := make([]byte, 20)
			putU32(truncatedTail[8:12], 0)
			putU32(truncatedTail[12:16], 0)
			putU32(truncatedTail[16:20], 0)

			for _, encoded := range [][]byte{shortRecord, truncatedCommand, truncatedTool, truncatedDispatch, truncatedTail} {
				_, err := decodeRunRecord(encoded)
				Expect(err).To(HaveOccurred())
			}
		})

		DescribeTable("rejects encoded records truncated at exact field boundaries",
			func(chop int) {
				encoded := encodeRunRecord(runRecord{
					TimestampUnix: 1_700_000_123,
					Command:       "cmd",
					Tool:          "go",
					Dispatch:      "go:test",
					RawBytes:      12,
					KeptBytes:     4,
					ExitCode:      2,
					DurationMS:    25,
					Passthrough:   true,
				})

				_, err := decodeRunRecord(encoded[:len(encoded)-chop])
				Expect(err).To(HaveOccurred())
			},
			Entry("when the command payload is short by one byte", 1+8+8+8+8+1+len("go:test")+4+len("go")+4),
			Entry("when the tool payload is short by one byte", 1+8+8+8+8+1+len("go:test")+4),
			Entry("when the dispatch payload is short by one byte", 1+8+8+8+8+1),
			Entry("when the fixed-width tail is short by one byte", 1),
		)

		DescribeTable("truncates commands only when they exceed the rune limit",
			func(cmd string, expected string) {
				Expect(truncateCommand(cmd)).To(Equal(expected))
			},
			Entry("keeps exact-limit commands unchanged", strings.Repeat("x", maxCommandTextRunes), strings.Repeat("x", maxCommandTextRunes)),
			Entry("truncates commands longer than the limit", strings.Repeat("x", maxCommandTextRunes+1), strings.Repeat("x", maxCommandTextRunes-3)+"..."),
		)

		DescribeTable("matches query options exactly",
			func(rec runRecord, opts QueryOptions, sinceUnix int64, expected bool) {
				Expect(matchesOptions(rec, opts, sinceUnix)).To(Equal(expected))
			},
			Entry("accepts a record when all filters match", runRecord{TimestampUnix: 200, Tool: "go", ExitCode: 1}, QueryOptions{Tool: "go", Failed: true}, int64(100), true),
			Entry("rejects older records when since is active", runRecord{TimestampUnix: 99, Tool: "go"}, QueryOptions{}, int64(100), false),
			Entry("rejects non-matching tools", runRecord{TimestampUnix: 200, Tool: "git"}, QueryOptions{Tool: "go"}, int64(0), false),
			Entry("rejects successful commands when failed-only is requested", runRecord{TimestampUnix: 200, Tool: "go", ExitCode: 0}, QueryOptions{Failed: true}, int64(0), false),
		)

		DescribeTable("builds period buckets exactly",
			func(ts time.Time, period string, expectedBucket string, expectedStart string, expectedEnd string, wantErr string) {
				bucket, start, end, err := bucketFor(ts, period)
				if wantErr != "" {
					Expect(err).To(MatchError(wantErr))
					return
				}

				Expect(err).NotTo(HaveOccurred())
				Expect(bucket).To(Equal(expectedBucket))
				Expect(start).To(Equal(expectedStart))
				Expect(end).To(Equal(expectedEnd))
			},
			Entry("groups by day", time.Date(2026, 3, 26, 14, 0, 0, 0, time.UTC), "day", "2026-03-26", "2026-03-26", "2026-03-26", ""),
			Entry("groups by week using ISO boundaries", time.Date(2026, 1, 1, 14, 0, 0, 0, time.UTC), "week", "2026-W01", "2025-12-29", "2026-01-04", ""),
			Entry("groups by month using month boundaries", time.Date(2024, 2, 20, 14, 0, 0, 0, time.UTC), "month", "2024-02", "2024-02-01", "2024-02-29", ""),
			Entry("rejects invalid periods", time.Date(2026, 3, 26, 14, 0, 0, 0, time.UTC), "quarter", "", "", "", `invalid period "quarter"`),
		)

		It("round-trips encoded run records and defaults blank tools to unknown", func() {
			rec := runRecord{
				TimestampUnix:         1_700_000_123,
				Command:               metricsGoTestCommand,
				Tool:                  "",
				Dispatch:              "go:test",
				RawBytes:              12,
				KeptBytes:             4,
				ExitCode:              -3,
				DurationMS:            17,
				Passthrough:           true,
				FilterSourceKind:      "project",
				FilterPath:            "/repo/.cmdshape/filters/go.yaml",
				FilterHash:            "abc123",
				RegistryBuildRecorded: true,
				RegistryBuildMS:       42,
				RegistrySources: []RegistrySourceBuildMetric{
					{SourceKind: "project", SourceDir: "/repo/.cmdshape/filters", Definitions: 2, Compiled: 1, DurationMS: 12},
				},
			}

			got, err := decodeRunRecord(encodeRunRecord(rec))
			Expect(err).NotTo(HaveOccurred())
			Expect(got.TimestampUnix).To(Equal(rec.TimestampUnix))
			Expect(got.Command).To(Equal(rec.Command))
			Expect(got.Tool).To(Equal("unknown"))
			Expect(got.Dispatch).To(Equal(rec.Dispatch))
			Expect(got.RawBytes).To(Equal(rec.RawBytes))
			Expect(got.KeptBytes).To(Equal(rec.KeptBytes))
			Expect(got.ExitCode).To(Equal(rec.ExitCode))
			Expect(got.DurationMS).To(Equal(rec.DurationMS))
			Expect(got.Passthrough).To(BeTrue())
			Expect(got.FilterSourceKind).To(Equal("project"))
			Expect(got.FilterPath).To(Equal("/repo/.cmdshape/filters/go.yaml"))
			Expect(got.FilterHash).To(Equal("abc123"))
			Expect(got.RegistryBuildRecorded).To(BeTrue())
			Expect(got.RegistryBuildMS).To(Equal(int64(42)))
			Expect(got.RegistrySources).To(Equal([]RegistrySourceBuildMetric{
				{SourceKind: "project", SourceDir: "/repo/.cmdshape/filters", Definitions: 2, Compiled: 1, DurationMS: 12},
			}))
		})

		It("decodes legacy run records without provenance fields", func() {
			rec := runRecord{
				TimestampUnix: 1_700_000_123,
				Command:       metricsGoTestCommand,
				Tool:          "go",
				Dispatch:      "go|test",
				RawBytes:      12,
				KeptBytes:     4,
				ExitCode:      0,
				DurationMS:    17,
				Passthrough:   true,
			}
			encoded := encodeRunRecord(rec)
			legacyEncoded := encoded[:len(encoded)-12]

			got, err := decodeRunRecord(legacyEncoded)
			Expect(err).NotTo(HaveOccurred())

			Expect(got.TimestampUnix).To(Equal(rec.TimestampUnix))
			Expect(got.Command).To(Equal(rec.Command))
			Expect(got.Tool).To(Equal(rec.Tool))
			Expect(got.Dispatch).To(Equal(rec.Dispatch))
			Expect(got.RawBytes).To(Equal(rec.RawBytes))
			Expect(got.KeptBytes).To(Equal(rec.KeptBytes))
			Expect(got.Passthrough).To(BeTrue())
			Expect(got.FilterSourceKind).To(BeEmpty())
			Expect(got.FilterPath).To(BeEmpty())
			Expect(got.FilterHash).To(BeEmpty())
		})

		It("rejects invalid booleans and trailing bytes", func() {
			encoded := encodeRunRecord(runRecord{Command: "cmd", Tool: "go"})
			encoded[len(encoded)-13] = 2
			_, err := decodeRunRecord(encoded)
			Expect(err).To(HaveOccurred())

			encoded = append(encodeRunRecord(runRecord{
				Command:               "cmd",
				Tool:                  "go",
				RegistryBuildRecorded: true,
			}), 0)
			_, err = decodeRunRecord(encoded)
			Expect(err).To(MatchError("trailing record bytes"))
		})
	})
})

func appendRunMetrics(path string, metrics ...RunMetric) {
	for _, metric := range metrics {
		Expect(Append(path, metric)).To(Succeed())
	}
}

func withMetricsWorkingDir(dir string) {
	oldWd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	Expect(os.Chdir(dir)).To(Succeed())
	DeferCleanup(func() { _ = os.Chdir(oldWd) })
}

func initGitProjectForMetrics(project string, gitignoreContent string) string {
	Expect(os.Mkdir(filepath.Join(project, ".git"), 0o755)).To(Succeed())
	if gitignoreContent != "" {
		Expect(os.WriteFile(filepath.Join(project, gitignoreFileName), []byte(gitignoreContent), 0o644)).To(Succeed())
	}
	return project
}

func metricsGitCheckIgnore(root string, paths ...string) []string {
	args := append([]string{"-C", root, "check-ignore"}, paths...)
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() == 1 {
			return nil
		}
		Expect(err).NotTo(HaveOccurred())
	}
	var ignored []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			ignored = append(ignored, line)
		}
	}
	return ignored
}

func encodedU64(v uint64) []byte {
	dst := make([]byte, 8)
	putU64(dst, v)
	return dst
}
