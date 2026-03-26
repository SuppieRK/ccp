package metrics

import (
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	bolt "go.etcd.io/bbolt"
)

const (
	testGainDBPath = "gain.db"
	goTestCommand  = "go test ./..."
	periodDay      = "day"
	periodWeek     = "week"
	periodMonth    = "month"
)

var _ = Describe("metrics queries", func() {
	var (
		path string
		now  time.Time
	)

	BeforeEach(func() {
		path = filepath.Join(GinkgoT().TempDir(), testGainDBPath)
		now = time.Now().UTC()
	})

	It("bootstraps the schema", func() {
		Expect(Bootstrap(path)).To(Succeed())

		db, err := bolt.Open(path, 0o600, &bolt.Options{ReadOnly: true, Timeout: 50 * time.Millisecond})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = db.Close() })
		Expect(db.View(func(tx *bolt.Tx) error {
			Expect(tx.Bucket(runsBucket)).NotTo(BeNil())
			return nil
		})).To(Succeed())
	})

	It("times out when the database is locked", func() {
		Expect(Bootstrap(path)).To(Succeed())

		lockDB, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = lockDB.Close() })

		err = Append(path, RunMetric{
			Tool:      "go",
			Command:   goTestCommand,
			RawBytes:  10,
			KeptBytes: 5,
			ExitCode:  0,
		})
		Expect(err).To(HaveOccurred())
		Expect(IsTimeoutOrBusy(err)).To(BeTrue())
	})

	Context("with seeded query data", func() {
		BeforeEach(func() {
			appendSeedMetrics(path, queryFilterSeed(now))
		})

		It("filters summary rows by tool", func() {
			rows, err := QuerySummaryRows(path, QueryOptions{Tool: "go"})
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(1))
			Expect(rows[0].Command).To(Equal(goTestCommand))
		})

		It("summarizes rows by tool", func() {
			toolRows, err := QuerySummaryRowsByTool(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(toolRows).To(HaveLen(2))
			Expect(toolRows[0].Tool).To(Equal("go"))
			Expect(toolRows[0].Commands).To(Equal(int64(2)))
			Expect(toolRows[0].EstimatedInputTokens).To(Equal(int64((2000 + 3) / 4)))
			Expect(toolRows[0].EstimatedOutputTokens).To(Equal(int64((700 + 3) / 4)))
		})

		It("filters failed history rows", func() {
			failedRows, err := QueryHistory(path, QueryOptions{Failed: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(failedRows).To(HaveLen(1))
			Expect(failedRows[0].ExitCode).NotTo(BeZero())
		})

		It("filters history rows by recent window", func() {
			sinceRows, err := QueryHistory(path, QueryOptions{Since: 3 * time.Hour})
			Expect(err).NotTo(HaveOccurred())
			Expect(sinceRows).To(HaveLen(2))
		})

		DescribeTable("returns rows for supported periods",
			func(period string) {
				out, err := QueryPeriod(path, QueryOptions{Period: period})
				Expect(err).NotTo(HaveOccurred())
				Expect(out).NotTo(BeEmpty())
			},
			Entry("day", periodDay),
			Entry("week", periodWeek),
			Entry("month", periodMonth),
		)

		It("returns missed opportunities ordered by count", func() {
			missed, err := QueryMissedOpportunities(path, QueryOptions{}, 5)
			Expect(err).NotTo(HaveOccurred())
			Expect(missed).NotTo(BeEmpty())
			Expect(missed[0].Count >= missed[len(missed)-1].Count).To(BeTrue())
		})

		It("orders history rows newest first", func() {
			history, err := QueryHistory(path, QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(HaveLen(3))
			Expect(history[0].Timestamp.After(history[1].Timestamp)).To(BeTrue())
			Expect(history[1].Timestamp.After(history[2].Timestamp)).To(BeTrue())
		})
	})

	It("uses Monday-Sunday weekly buckets", func() {
		appendSeedMetrics(path, []RunMetric{
			{Timestamp: time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC), Tool: "go", Command: goTestCommand, RawBytes: 100, KeptBytes: 40},
			{Timestamp: time.Date(2026, 3, 8, 22, 0, 0, 0, time.UTC), Tool: "go", Command: goTestCommand, RawBytes: 200, KeptBytes: 80},
		})

		rows, err := QueryPeriod(path, QueryOptions{Period: periodWeek})
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).To(HaveLen(1))
		Expect(rows[0].BucketStart).To(Equal("2026-03-02"))
		Expect(rows[0].BucketEnd).To(Equal("2026-03-08"))
	})

	It("filters period queries by recent windows", func() {
		appendSeedMetrics(path, []RunMetric{
			{Timestamp: now.Add(-10 * 24 * time.Hour), Tool: "go", Command: goTestCommand, RawBytes: 100, KeptBytes: 40},
			{Timestamp: now.Add(-6 * 24 * time.Hour), Tool: "go", Command: goTestCommand, RawBytes: 200, KeptBytes: 80},
			{Timestamp: now.Add(-2 * 24 * time.Hour), Tool: "git", Command: "git status", RawBytes: 120, KeptBytes: 120},
		})

		rows, err := QueryPeriod(path, QueryOptions{Since: 7 * 24 * time.Hour, Period: periodDay})
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).To(HaveLen(2))
		for _, row := range rows {
			Expect(row.BucketStart).NotTo(Equal(now.Add(-10 * 24 * time.Hour).Format("2006-01-02")))
		}
	})

	It("returns an error for unsupported periods", func() {
		appendSeedMetrics(path, []RunMetric{
			{Timestamp: now.Add(-time.Hour), Tool: "go", Command: goTestCommand, RawBytes: 100, KeptBytes: 40},
		})

		_, err := QueryPeriod(path, QueryOptions{Period: "year"})
		Expect(err).To(MatchError(`invalid period "year"`))
	})

	It("orders summary rows by command frequency then command text", func() {
		appendSeedMetrics(path, []RunMetric{
			{Timestamp: now.Add(-4 * time.Hour), Tool: "go", Command: "go build ./...", RawBytes: 500, KeptBytes: 200},
			{Timestamp: now.Add(-3 * time.Hour), Tool: "go", Command: "go build ./...", RawBytes: 500, KeptBytes: 200},
			{Timestamp: now.Add(-2 * time.Hour), Tool: "go", Command: "go test ./...", RawBytes: 600, KeptBytes: 250},
			{Timestamp: now.Add(-time.Hour), Tool: "go", Command: "go test ./...", RawBytes: 600, KeptBytes: 250},
			{Timestamp: now.Add(-30 * time.Minute), Tool: "go", Command: "go clean ./...", RawBytes: 100, KeptBytes: 100},
		})

		rows, err := QuerySummaryRows(path, QueryOptions{Tool: "go"})
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).To(HaveLen(3))
		Expect(rows[0].Command).To(Equal("go build ./..."))
		Expect(rows[0].Commands).To(Equal(int64(2)))
		Expect(rows[1].Command).To(Equal(goTestCommand))
		Expect(rows[1].Commands).To(Equal(int64(2)))
		Expect(rows[2].Command).To(Equal("go clean ./..."))
		Expect(rows[2].Commands).To(Equal(int64(1)))
	})

	It("orders tool summaries by command frequency then tool name", func() {
		appendSeedMetrics(path, []RunMetric{
			{Timestamp: now.Add(-4 * time.Hour), Tool: "go", Command: goTestCommand, RawBytes: 500, KeptBytes: 200},
			{Timestamp: now.Add(-3 * time.Hour), Tool: "go", Command: "go build ./...", RawBytes: 500, KeptBytes: 200},
			{Timestamp: now.Add(-2 * time.Hour), Tool: "git", Command: "git status", RawBytes: 120, KeptBytes: 120},
			{Timestamp: now.Add(-time.Hour), Tool: "git", Command: "git branch", RawBytes: 120, KeptBytes: 120},
			{Timestamp: now.Add(-30 * time.Minute), Tool: "node", Command: "node script.js", RawBytes: 200, KeptBytes: 80},
		})

		rows, err := QuerySummaryRowsByTool(path, QueryOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).To(HaveLen(3))
		Expect(rows[0].Tool).To(Equal("git"))
		Expect(rows[0].Commands).To(Equal(int64(2)))
		Expect(rows[1].Tool).To(Equal("go"))
		Expect(rows[1].Commands).To(Equal(int64(2)))
		Expect(rows[2].Tool).To(Equal("node"))
		Expect(rows[2].Commands).To(Equal(int64(1)))
	})

	It("uses the default missed opportunity limit for non-positive limits", func() {
		appendSeedMetrics(path, []RunMetric{
			{Timestamp: now.Add(-7 * time.Hour), Tool: "go", Command: "go test ./a", RawBytes: 100, KeptBytes: 100, Passthrough: true},
			{Timestamp: now.Add(-6 * time.Hour), Tool: "go", Command: "go test ./a", RawBytes: 100, KeptBytes: 100, Passthrough: true},
			{Timestamp: now.Add(-5 * time.Hour), Tool: "go", Command: "go test ./b", RawBytes: 100, KeptBytes: 100, Passthrough: true},
			{Timestamp: now.Add(-4 * time.Hour), Tool: "go", Command: "go test ./b", RawBytes: 100, KeptBytes: 100, Passthrough: true},
			{Timestamp: now.Add(-3 * time.Hour), Tool: "go", Command: "go test ./c", RawBytes: 100, KeptBytes: 100, Passthrough: true},
			{Timestamp: now.Add(-2 * time.Hour), Tool: "go", Command: "go test ./d", RawBytes: 100, KeptBytes: 100, Passthrough: true},
			{Timestamp: now.Add(-time.Hour), Tool: "go", Command: "go test ./e", RawBytes: 100, KeptBytes: 100, Passthrough: true},
			{Timestamp: now.Add(-30 * time.Minute), Tool: "go", Command: "go test ./f", RawBytes: 100, KeptBytes: 100, Passthrough: true},
		})

		rows, err := QueryMissedOpportunities(path, QueryOptions{}, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).To(HaveLen(5))
		Expect(rows[0]).To(Equal(MissedOpportunity{Command: "go test ./a", Count: 2}))
		Expect(rows[1]).To(Equal(MissedOpportunity{Command: "go test ./b", Count: 2}))
		Expect(rows[2].Command).To(Equal("go test ./c"))
		Expect(rows[3].Command).To(Equal("go test ./d"))
		Expect(rows[4].Command).To(Equal("go test ./e"))
	})

	It("keeps summary totals consistent with summary rows", func() {
		appendSeedMetrics(path, []RunMetric{
			{Timestamp: now.Add(-90 * time.Minute), Tool: "go", Command: "go test ./...", RawBytes: 1000, KeptBytes: 500},
			{Timestamp: now.Add(-80 * time.Minute), Tool: "go", Command: "go build ./...", RawBytes: 400, KeptBytes: 300},
			{Timestamp: now.Add(-70 * time.Minute), Tool: "git", Command: "git status", RawBytes: 200, KeptBytes: 200},
		})

		opts := QueryOptions{Tool: "go"}
		rows, err := QuerySummaryRows(path, opts)
		Expect(err).NotTo(HaveOccurred())
		total, err := QuerySummary(path, opts)
		Expect(err).NotTo(HaveOccurred())

		var sumCommands, sumRaw, sumKept int64
		for _, r := range rows {
			sumCommands += r.Commands
			sumRaw += r.RawBytes
			sumKept += r.KeptBytes
		}
		Expect(sumCommands).To(Equal(total.Commands))
		Expect(sumRaw).To(Equal(total.RawBytes))
		Expect(sumKept).To(Equal(total.KeptBytes))
	})

	It("truncates commands as prefix plus ellipsis", func() {
		long := strings.Repeat("x", 1500)
		Expect(Append(path, RunMetric{Tool: "go", Command: long, RawBytes: 100, KeptBytes: 10})).To(Succeed())

		history, err := QueryHistory(path, QueryOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(HaveLen(1))
		got := history[0].Command
		Expect(len([]rune(got))).To(Equal(1024))
		Expect(got).To(Equal(strings.Repeat("x", 1021) + "..."))
	})

	It("uses ceil(bytes/4) token estimates", func() {
		Expect(Append(path, RunMetric{
			Tool: "go", Command: "go test ./...", RawBytes: 5, KeptBytes: 1,
		})).To(Succeed())

		rows, err := QueryHistory(path, QueryOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).To(HaveLen(1))
		Expect(rows[0].EstimatedInputTokens).To(Equal(int64(2)))
		Expect(rows[0].EstimatedOutputTokens).To(Equal(int64(1)))
		Expect(rows[0].EstimatedSavedTokens).To(Equal(int64(1)))
	})
})

func queryFilterSeed(now time.Time) []RunMetric {
	return []RunMetric{
		{
			Timestamp:   now.Add(-48 * time.Hour),
			Tool:        "go",
			Command:     goTestCommand,
			RawBytes:    1200,
			KeptBytes:   400,
			ExitCode:    0,
			Passthrough: false,
		},
		{
			Timestamp:   now.Add(-2 * time.Hour),
			Tool:        "go",
			Command:     goTestCommand,
			RawBytes:    800,
			KeptBytes:   300,
			ExitCode:    1,
			Passthrough: true,
		},
		{
			Timestamp:   now.Add(-1 * time.Hour),
			Tool:        "git",
			Command:     "git status",
			RawBytes:    500,
			KeptBytes:   500,
			ExitCode:    0,
			Passthrough: true,
		},
	}
}

func appendSeedMetrics(path string, seed []RunMetric) {
	for _, m := range seed {
		Expect(Append(path, m)).To(Succeed())
	}
}
