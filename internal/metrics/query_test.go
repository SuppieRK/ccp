package metrics

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	testGainDBPath = "gain.db"
	goTestCommand  = "go test ./..."
	periodDay      = "day"
	periodWeek     = "week"
	periodMonth    = "month"
)

func TestBootstrapCreatesSchema(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), testGainDBPath)
	if err := Bootstrap(path); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{ReadOnly: true, Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("close db: %v", err)
		}
	})
	if err := db.View(func(tx *bolt.Tx) error {
		if tx.Bucket(runsBucket) == nil {
			t.Fatalf("runs bucket missing")
		}
		return nil
	}); err != nil {
		t.Fatalf("view db: %v", err)
	}
}

func TestAppendTimesOutWhenDatabaseIsLocked(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), testGainDBPath)
	if err := Bootstrap(path); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	lockDB, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		t.Fatalf("open lock db: %v", err)
	}
	t.Cleanup(func() {
		if err := lockDB.Close(); err != nil {
			t.Fatalf("close lock db: %v", err)
		}
	})

	err = Append(path, RunMetric{
		Tool:      "go",
		Command:   goTestCommand,
		RawBytes:  10,
		KeptBytes: 5,
		ExitCode:  0,
	})
	if err == nil {
		t.Fatalf("expected append error while db is locked")
	}
	if !IsTimeoutOrBusy(err) {
		t.Fatalf("expected timeout/busy classification, got: %v", err)
	}
}

func TestQueryFiltersPeriodAndMissedOpportunities(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), testGainDBPath)
	now := time.Now().UTC()
	appendSeedMetrics(t, path, queryFilterSeed(now))
	assertToolFilteredSummaryRows(t, path)
	assertSummaryRowsByTool(t, path)
	assertFailedHistoryRows(t, path)
	assertSinceFilteredRows(t, path, 3*time.Hour, 2)
	assertPeriodQueriesReturnRows(t, path, periodDay, periodWeek, periodMonth)
	assertMissedOpportunities(t, path)
}

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

func assertToolFilteredSummaryRows(t *testing.T, path string) {
	t.Helper()
	rows, err := QuerySummaryRows(path, QueryOptions{Tool: "go"})
	if err != nil {
		t.Fatalf("summary rows by tool: %v", err)
	}
	if len(rows) != 1 || rows[0].Command != goTestCommand {
		t.Fatalf("unexpected tool-filtered rows: %#v", rows)
	}
}

func assertSummaryRowsByTool(t *testing.T, path string) {
	t.Helper()
	toolRows, err := QuerySummaryRowsByTool(path, QueryOptions{})
	if err != nil {
		t.Fatalf("summary rows by tool aggregate: %v", err)
	}
	if len(toolRows) != 2 {
		t.Fatalf("tool rows = %d, want 2", len(toolRows))
	}
	if toolRows[0].Tool != "go" || toolRows[0].Commands != 2 {
		t.Fatalf("unexpected first tool aggregate row: %#v", toolRows[0])
	}
	if toolRows[0].EstimatedInputTokens != ((2000+3)/4) || toolRows[0].EstimatedOutputTokens != ((700+3)/4) {
		t.Fatalf("unexpected token estimates from bytes for go aggregate: %#v", toolRows[0])
	}
}

func assertFailedHistoryRows(t *testing.T, path string) {
	t.Helper()
	failedRows, err := QueryHistory(path, QueryOptions{Failed: true})
	if err != nil {
		t.Fatalf("history failed filter: %v", err)
	}
	if len(failedRows) != 1 || failedRows[0].ExitCode == 0 {
		t.Fatalf("unexpected failed-only rows: %#v", failedRows)
	}
}

func assertSinceFilteredRows(t *testing.T, path string, since time.Duration, want int) {
	t.Helper()
	sinceRows, err := QueryHistory(path, QueryOptions{Since: since})
	if err != nil {
		t.Fatalf("history since filter: %v", err)
	}
	if len(sinceRows) != want {
		t.Fatalf("since-filter rows = %d, want %d", len(sinceRows), want)
	}
}

func assertPeriodQueriesReturnRows(t *testing.T, path string, periods ...string) {
	t.Helper()
	for _, p := range periods {
		out, err := QueryPeriod(path, QueryOptions{Period: p})
		if err != nil {
			t.Fatalf("period query (%s): %v", p, err)
		}
		if len(out) == 0 {
			t.Fatalf("period query (%s) returned no rows", p)
		}
	}
}

func assertMissedOpportunities(t *testing.T, path string) {
	t.Helper()
	missed, err := QueryMissedOpportunities(path, QueryOptions{}, 5)
	if err != nil {
		t.Fatalf("missed opportunities: %v", err)
	}
	if len(missed) == 0 {
		t.Fatalf("expected missed opportunities rows")
	}
	if missed[0].Count < missed[len(missed)-1].Count {
		t.Fatalf("expected descending sort by count: %#v", missed)
	}
}

func TestQueryPeriodWeekUsesMondayStartAndSundayEnd(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), testGainDBPath)
	// Thursday and Sunday in the same ISO week (Mon-start week: 2026-03-02..2026-03-08).
	seed := []RunMetric{
		{
			Timestamp: time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC),
			Tool:      "go", Command: goTestCommand, RawBytes: 100, KeptBytes: 40,
		},
		{
			Timestamp: time.Date(2026, 3, 8, 22, 0, 0, 0, time.UTC),
			Tool:      "go", Command: goTestCommand, RawBytes: 200, KeptBytes: 80,
		},
	}
	appendSeedMetrics(t, path, seed)

	rows, err := QueryPeriod(path, QueryOptions{Period: periodWeek})
	if err != nil {
		t.Fatalf("query period week: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("weekly rows = %d, want 1", len(rows))
	}
	if rows[0].BucketStart != "2026-03-02" || rows[0].BucketEnd != "2026-03-08" {
		t.Fatalf("unexpected week bounds: start=%s end=%s", rows[0].BucketStart, rows[0].BucketEnd)
	}
}

func TestQuerySummaryAndRowsUseEquivalentSelection(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), testGainDBPath)
	now := time.Now().UTC()
	seed := []RunMetric{
		{Timestamp: now.Add(-90 * time.Minute), Tool: "go", Command: "go test ./...", RawBytes: 1000, KeptBytes: 500},
		{Timestamp: now.Add(-80 * time.Minute), Tool: "go", Command: "go build ./...", RawBytes: 400, KeptBytes: 300},
		{Timestamp: now.Add(-70 * time.Minute), Tool: "git", Command: "git status", RawBytes: 200, KeptBytes: 200},
	}
	appendSeedMetrics(t, path, seed)

	opts := QueryOptions{Tool: "go"}
	rows, err := QuerySummaryRows(path, opts)
	if err != nil {
		t.Fatalf("query summary rows: %v", err)
	}
	total, err := QuerySummary(path, opts)
	if err != nil {
		t.Fatalf("query summary total: %v", err)
	}
	var sumCommands, sumRaw, sumKept int64
	for _, r := range rows {
		sumCommands += r.Commands
		sumRaw += r.RawBytes
		sumKept += r.KeptBytes
	}
	if sumCommands != total.Commands || sumRaw != total.RawBytes || sumKept != total.KeptBytes {
		t.Fatalf("selection mismatch rows vs total: rows=(%d,%d,%d) total=(%d,%d,%d)",
			sumCommands, sumRaw, sumKept, total.Commands, total.RawBytes, total.KeptBytes)
	}
}

func TestAppendTruncatesCommandAsPrefixPlusEllipsis(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), testGainDBPath)
	long := strings.Repeat("x", 1500)
	if err := Append(path, RunMetric{Tool: "go", Command: long, RawBytes: 100, KeptBytes: 10}); err != nil {
		t.Fatalf("append long command: %v", err)
	}
	history, err := QueryHistory(path, QueryOptions{})
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history rows = %d, want 1", len(history))
	}
	got := history[0].Command
	if len([]rune(got)) != 1024 {
		t.Fatalf("command length = %d, want 1024", len([]rune(got)))
	}
	prefix := strings.Repeat("x", 1021)
	if got != prefix+"..." {
		t.Fatalf("unexpected truncation result: len=%d suffix=%q", len([]rune(got)), got[len(got)-6:])
	}
}

func TestHistoryTokenEstimatesUseCeilBytesDiv4(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), testGainDBPath)
	if err := Append(path, RunMetric{
		Tool: "go", Command: "go test ./...", RawBytes: 5, KeptBytes: 1,
	}); err != nil {
		t.Fatalf("append metric: %v", err)
	}
	rows, err := QueryHistory(path, QueryOptions{})
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("history rows = %d, want 1", len(rows))
	}
	if rows[0].EstimatedInputTokens != 2 || rows[0].EstimatedOutputTokens != 1 || rows[0].EstimatedSavedTokens != 1 {
		t.Fatalf("unexpected ceil token estimates: %+v", rows[0])
	}
}

func appendSeedMetrics(t *testing.T, path string, seed []RunMetric) {
	t.Helper()
	for _, m := range seed {
		if err := Append(path, m); err != nil {
			t.Fatalf("append seed metric: %v", err)
		}
	}
}
