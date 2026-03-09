package lifecycle

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-command-compression-proxy/internal/metrics"
)

const (
	flagFormat = "--format"
	flagPeriod = "--period"
	flagSince  = "--since"
	flagTable  = "--table"
)

func TestRunGainJSON(t *testing.T) {
	path := seedGainDB(t)
	out := captureStdout(t, func() error {
		return RunGain([]string{"--json"}, path)
	})
	if !strings.Contains(out, `"dataset": "summary"`) || !strings.Contains(out, `"total"`) {
		t.Fatalf("unexpected gain json output: %q", out)
	}
}

func TestRunGainDefaultRenderingIsText(t *testing.T) {
	path := seedGainDB(t)
	out := captureStdout(t, func() error {
		return RunGain([]string{}, path)
	})
	if !strings.Contains(out, "ccp gain (estimated tokens: 4B/token)") {
		t.Fatalf("expected text renderer header, got %q", out)
	}
	if !strings.Contains(out, "Biggest gains:") || !strings.Contains(out, "Bottom line:") {
		t.Fatalf("expected shareable summary markers, got %q", out)
	}
}

func TestRunGainDefaultRenderingFormatsGroupedNumbersAndNoSavingsText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gain.db")
	now := time.Now().UTC()
	appendGainMetrics(t, path, []metrics.RunMetric{
		{
			Timestamp: now.Add(-2 * time.Hour),
			Tool:      "gradle",
			Command:   "./gradlew test",
			RawBytes:  20_000_000,
			KeptBytes: 100_000,
			ExitCode:  0,
		},
		{
			Timestamp: now.Add(-1 * time.Hour),
			Tool:      "jar",
			Command:   "jar tf app.jar",
			RawBytes:  8_000,
			KeptBytes: 8_000,
			ExitCode:  0,
		},
	})

	out := captureStdout(t, func() error {
		return RunGain([]string{}, path)
	})
	if !strings.Contains(out, "5,002,000") || !strings.Contains(out, "27,000") {
		t.Fatalf("expected grouped-number formatting in summary output, got %q", out)
	}
	if !strings.Contains(out, "jar (1 cmds, no savings)") {
		t.Fatalf("expected zero-savings phrasing in summary output, got %q", out)
	}
}

func TestRunGainTableIncludesCompactTableAndMissedOpportunities(t *testing.T) {
	path := seedGainDB(t)
	out := captureStdout(t, func() error {
		return RunGain([]string{flagTable}, path)
	})
	if !strings.Contains(out, "TOOL") || !strings.Contains(out, "NATIVE") || !strings.Contains(out, "PROXIED") || !strings.Contains(out, "SAVINGS") || !strings.Contains(out, "TOTAL") {
		t.Fatalf("missing summary table markers: %q", out)
	}
	if strings.Contains(out, "COMMAND") {
		t.Fatalf("expected tool-based summary header (no COMMAND column): %q", out)
	}
	if !strings.Contains(out, "go") || !strings.Contains(out, "git") {
		t.Fatalf("expected tool aggregates in text summary: %q", out)
	}
	if strings.Contains(out, "Missed opportunities") {
		t.Fatalf("unexpected missed opportunities section in summary text: %q", out)
	}
}

func TestRunGainTableAlignsTotalRowWithCountColumn(t *testing.T) {
	path := seedGainDB(t)
	out := captureStdout(t, func() error {
		return RunGain([]string{flagTable}, path)
	})

	lines := strings.Split(out, "\n")
	firstDataCountCol := -1
	totalCountCol := -1

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "ccp gain") || strings.HasPrefix(trimmed, "filters:") || strings.HasPrefix(trimmed, "TOOL") {
			continue
		}
		countCol := strings.IndexFunc(line, func(r rune) bool { return r >= '0' && r <= '9' })
		if countCol < 0 {
			continue
		}
		if strings.HasPrefix(strings.TrimLeft(line, " "), "TOTAL") {
			totalCountCol = countCol
			continue
		}
		if firstDataCountCol < 0 {
			firstDataCountCol = countCol
		}
	}

	if firstDataCountCol < 0 || totalCountCol < 0 {
		t.Fatalf("failed to locate summary rows in output:\n%s", out)
	}
	if firstDataCountCol != totalCountCol {
		t.Fatalf("expected TOTAL count column alignment, data=%d total=%d\noutput:\n%s", firstDataCountCol, totalCountCol, out)
	}
}

func TestRunGainTableFormatsGroupedNumbers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gain.db")
	now := time.Now().UTC()
	appendGainMetrics(t, path, []metrics.RunMetric{
		{
			Timestamp: now.Add(-2 * time.Hour),
			Tool:      "gradle",
			Command:   "./gradlew test",
			RawBytes:  20_000_000,
			KeptBytes: 100_000,
			ExitCode:  0,
		},
		{
			Timestamp: now.Add(-1 * time.Hour),
			Tool:      "jar",
			Command:   "jar tf app.jar",
			RawBytes:  8_000,
			KeptBytes: 8_000,
			ExitCode:  0,
		},
	})

	out := captureStdout(t, func() error {
		return RunGain([]string{flagTable}, path)
	})
	if !strings.Contains(out, "5,002,000") || !strings.Contains(out, "27,000") {
		t.Fatalf("expected grouped-number formatting in table output, got %q", out)
	}
}

func TestRunGainTableTieBreakOrderingCountThenNativeThenTool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gain.db")
	now := time.Now().UTC()
	// Three tools with same COUNT=1. "alpha" and "zeta" tie on NATIVE and must sort by tool asc.
	seed := []metrics.RunMetric{
		{Timestamp: now.Add(-3 * time.Minute), Tool: "alpha", Command: "alpha cmd", RawBytes: 200, KeptBytes: 100},
		{Timestamp: now.Add(-2 * time.Minute), Tool: "zeta", Command: "zeta cmd", RawBytes: 200, KeptBytes: 100},
		{Timestamp: now.Add(-1 * time.Minute), Tool: "middle", Command: "middle cmd", RawBytes: 120, KeptBytes: 60},
	}
	appendGainMetrics(t, path, seed)

	out := captureStdout(t, func() error {
		return RunGain([]string{flagTable}, path)
	})
	alphaIdx := strings.Index(out, "alpha")
	zetaIdx := strings.Index(out, "zeta")
	middleIdx := strings.Index(out, "middle")
	if alphaIdx < 0 || zetaIdx < 0 || middleIdx < 0 {
		t.Fatalf("expected tool rows in output, got %q", out)
	}
	// Higher NATIVE comes first: alpha/zeta (50) before middle (30).
	if alphaIdx > middleIdx || zetaIdx > middleIdx {
		t.Fatalf("expected higher-native rows before lower-native row, got %q", out)
	}
	// Equal COUNT and NATIVE tie-break by tool asc: alpha before zeta.
	if alphaIdx > zetaIdx {
		t.Fatalf("expected alpha row before zeta row for tie-break, got %q", out)
	}
}

func TestRunHistoryJSONAndCSV(t *testing.T) {
	path := seedGainDB(t)
	jsonOut := captureStdout(t, func() error {
		return RunHistory([]string{flagFormat, "json"}, path)
	})
	if !strings.Contains(jsonOut, `"dataset": "history"`) {
		t.Fatalf("unexpected history json: %q", jsonOut)
	}
	csvOut := captureStdout(t, func() error {
		return RunHistory([]string{flagFormat, "csv"}, path)
	})
	if !strings.Contains(csvOut, "dataset,period,since,tool_filter") || !strings.Contains(csvOut, "history") {
		t.Fatalf("unexpected history csv: %q", csvOut)
	}
}

func TestRunHistoryAppliesSharedFiltersAndNewestFirstOrder(t *testing.T) {
	path := seedGainDBWithFailure(t)

	out := captureStdout(t, func() error {
		return RunHistory([]string{flagFormat, "json", "--tool", "git", "--failed"}, path)
	})

	var env historyEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("unmarshal history envelope: %v", err)
	}
	if !env.Filters.Failed || env.Filters.Tool != "git" {
		t.Fatalf("unexpected filters envelope: %+v", env.Filters)
	}
	if len(env.Rows) != 2 {
		t.Fatalf("expected 2 filtered rows, got %d (%+v)", len(env.Rows), env.Rows)
	}
	for _, row := range env.Rows {
		if row.Tool != "git" || !row.Failed {
			t.Fatalf("unexpected filtered row: %+v", row)
		}
	}
	if env.Rows[0].Timestamp.Before(env.Rows[1].Timestamp) {
		t.Fatalf("expected newest-first ordering, got row0=%s row1=%s", env.Rows[0].Timestamp, env.Rows[1].Timestamp)
	}
}

func TestRunGainCSVAndPeriodFormats(t *testing.T) {
	path := seedGainDB(t)
	csvOut := captureStdout(t, func() error {
		return RunGain([]string{flagFormat, "csv"}, path)
	})
	if !strings.Contains(csvOut, "dataset,period,since,tool_filter") || !strings.Contains(csvOut, "summary") {
		t.Fatalf("unexpected gain csv: %q", csvOut)
	}

	periodText := captureStdout(t, func() error {
		return RunGain([]string{flagPeriod, "day"}, path)
	})
	if !strings.Contains(periodText, "period=day") || !strings.Contains(periodText, "Last 24h:") || !strings.Contains(periodText, "Biggest gains:") {
		t.Fatalf("unexpected gain period text: %q", periodText)
	}

	periodTable := captureStdout(t, func() error {
		return RunGain([]string{flagPeriod, "day", flagTable}, path)
	})
	if !strings.Contains(periodTable, "BUCKET") || !strings.Contains(periodTable, "period=day") {
		t.Fatalf("unexpected gain period table text: %q", periodTable)
	}

	periodCSV := captureStdout(t, func() error {
		return RunGain([]string{flagPeriod, "week", flagFormat, "csv"}, path)
	})
	if !strings.Contains(periodCSV, "dataset,period,since,tool_filter,failed_filter,bucket") || !strings.Contains(periodCSV, "period,week") {
		t.Fatalf("unexpected gain period csv: %q", periodCSV)
	}
}

func TestRunHistoryTextOmitsToolColumn(t *testing.T) {
	path := seedGainDB(t)
	out := captureStdout(t, func() error {
		return RunHistory([]string{flagFormat, "text"}, path)
	})
	if !strings.Contains(out, "TIMESTAMP") || !strings.Contains(out, "STATUS") || !strings.Contains(out, "SAVINGS") {
		t.Fatalf("missing history text headers: %q", out)
	}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "TIMESTAMP") {
			if strings.Contains(line, "TOOL") {
				t.Fatalf("history text header should omit TOOL: %q", line)
			}
			break
		}
	}
}

func TestRunGainWeekSummaryHighlightsBestAndBusiestDays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gain.db")
	now := time.Now().UTC().Truncate(time.Hour)
	bestDay := now.Add(-6*24*time.Hour + 9*time.Hour).Format("2006-01-02")
	busiestDay := now.Add(-4*24*time.Hour + 9*time.Hour).Format("2006-01-02")
	seed := []metrics.RunMetric{
		{Timestamp: now.Add(-6*24*time.Hour + 9*time.Hour), Tool: "go", Command: "go test ./...", RawBytes: 1200, KeptBytes: 200},
		{Timestamp: now.Add(-6*24*time.Hour + 10*time.Hour), Tool: "git", Command: "git status", RawBytes: 400, KeptBytes: 300},
		{Timestamp: now.Add(-4*24*time.Hour + 9*time.Hour), Tool: "grep", Command: "grep -r needle .", RawBytes: 800, KeptBytes: 200},
		{Timestamp: now.Add(-4*24*time.Hour + 10*time.Hour), Tool: "sed", Command: "sed -n 1,20p file", RawBytes: 100, KeptBytes: 100},
		{Timestamp: now.Add(-4*24*time.Hour + 11*time.Hour), Tool: "git", Command: "git diff", RawBytes: 600, KeptBytes: 500},
	}
	appendGainMetrics(t, path, seed)

	out := captureStdout(t, func() error {
		return RunGain([]string{flagPeriod, "week"}, path)
	})
	if !strings.Contains(out, "Last 7d:") || !strings.Contains(out, "Busiest day: "+busiestDay) || !strings.Contains(out, "Best day: "+bestDay) || !strings.Contains(out, "Recent trend:") {
		t.Fatalf("unexpected weekly summary output: %q", out)
	}
}

func TestBottomLineMessageUsesFriendlyBands(t *testing.T) {
	cases := []struct {
		pct  float64
		want string
	}{
		{0, "This is fine, better opportunities will come."},
		{10, "It ain't much, but it's honest work."},
		{30, "Pretty decent for the noise that adds up all day."},
		{50, "A solid result, and less noise to drag around."},
		{70, "Now we're talking - much less noise to drag around."},
		{90, "Breathtaking results, with plenty of context back."},
	}
	for _, tc := range cases {
		if got := bottomLineMessage(tc.pct); got != tc.want {
			t.Fatalf("bottomLineMessage(%.2f) = %q, want %q", tc.pct, got, tc.want)
		}
	}
}

func TestRunGainInvalidFlags(t *testing.T) {
	path := seedGainDB(t)
	if RunGain([]string{flagFormat, "xml"}, path) == nil {
		t.Fatal("expected invalid format error")
	}
	if RunGain([]string{flagSince, "nope"}, path) == nil {
		t.Fatal("expected invalid since error")
	}
	if err := RunGain([]string{flagSince, "2d"}, path); err != nil {
		t.Fatalf("expected day shorthand to parse, got: %v", err)
	}
	if err := RunGain([]string{flagSince, "1w"}, path); err != nil {
		t.Fatalf("expected week shorthand to parse, got: %v", err)
	}
	if RunGain([]string{flagFormat, "json", flagTable}, path) == nil {
		t.Fatal("expected --table with json to fail")
	}
}

func TestRunHistoryRejectsPeriodFlag(t *testing.T) {
	path := seedGainDB(t)
	if err := RunHistory([]string{flagPeriod, "day"}, path); err == nil || !strings.Contains(err.Error(), "--period is only valid for gain") {
		t.Fatalf("expected history period rejection, got: %v", err)
	}
	if err := RunHistory([]string{flagTable}, path); err == nil || !strings.Contains(err.Error(), "--table is only valid for gain") {
		t.Fatalf("expected history table rejection, got: %v", err)
	}
}

func TestRunGainHumanEmptyAndRemovableDB(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "gain.db")
	if err := RunGain([]string{}, path); err != nil {
		t.Fatalf("run gain on empty db: %v", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove gain file: %v", err)
	}
}

func TestRunGainAndHistoryTextEmptyIncludeNoResultsAndFilters(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "gain.db")

	gainOut := captureStdout(t, func() error {
		return RunGain([]string{flagFormat, "text"}, path)
	})
	if !strings.Contains(gainOut, "filters:") || !strings.Contains(gainOut, noResultsMsg) {
		t.Fatalf("expected gain empty text to include filters and no-results message, got %q", gainOut)
	}

	historyOut := captureStdout(t, func() error {
		return RunHistory([]string{flagFormat, "text"}, path)
	})
	if !strings.Contains(historyOut, "filters:") || !strings.Contains(historyOut, noResultsMsg) {
		t.Fatalf("expected history empty text to include filters and no-results message, got %q", historyOut)
	}
}

func seedGainDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gain.db")
	now := time.Now().UTC()
	seed := []metrics.RunMetric{
		{
			Timestamp:   now.Add(-2 * time.Hour),
			Tool:        "go",
			Command:     "go test ./...",
			RawBytes:    1200,
			KeptBytes:   400,
			ExitCode:    0,
			Passthrough: false,
		},
		{
			Timestamp:   now.Add(-1 * time.Hour),
			Tool:        "git",
			Command:     "git push origin main",
			RawBytes:    500,
			KeptBytes:   500,
			ExitCode:    0,
			Passthrough: true,
		},
	}
	appendGainMetrics(t, path, seed)
	return path
}

func seedGainDBWithFailure(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gain.db")
	now := time.Now().UTC()
	seed := []metrics.RunMetric{
		{
			Timestamp:   now.Add(-3 * time.Hour),
			Tool:        "go",
			Command:     "go test ./...",
			RawBytes:    1200,
			KeptBytes:   400,
			ExitCode:    0,
			Passthrough: false,
		},
		{
			Timestamp:   now.Add(-2 * time.Hour),
			Tool:        "git",
			Command:     "git push origin main",
			RawBytes:    500,
			KeptBytes:   500,
			ExitCode:    1,
			Passthrough: true,
		},
		{
			Timestamp:   now.Add(-1 * time.Hour),
			Tool:        "git",
			Command:     "git pull origin main",
			RawBytes:    450,
			KeptBytes:   450,
			ExitCode:    2,
			Passthrough: true,
		},
	}
	appendGainMetrics(t, path, seed)
	return path
}

func appendGainMetrics(t *testing.T, path string, seed []metrics.RunMetric) {
	t.Helper()
	for _, m := range seed {
		if err := metrics.Append(path, m); err != nil {
			t.Fatalf("append metric: %v", err)
		}
	}
}

func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = orig
	}()

	runErr := fn()
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	if runErr != nil {
		t.Fatalf("run function: %v", runErr)
	}
	return buf.String()
}

func TestTruncateForDisplayBranches(t *testing.T) {
	if got := truncateForDisplay("abcdef", 0); got != "" {
		t.Fatalf("max<=0 expected empty, got %q", got)
	}
	if got := truncateForDisplay("abcdef", 3); got != "abc" {
		t.Fatalf("max<=3 expected hard cut, got %q", got)
	}
	if got := truncateForDisplay("abcdef", 5); got != "ab..." {
		t.Fatalf("expected ellipsis truncation, got %q", got)
	}
}
