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
	if !strings.Contains(out, "TOOL") || !strings.Contains(out, "TOTAL") {
		t.Fatalf("expected text summary table, got %q", out)
	}
}

func TestRunGainTextIncludesCompactTableAndMissedOpportunities(t *testing.T) {
	path := seedGainDB(t)
	out := captureStdout(t, func() error {
		return RunGain([]string{flagFormat, "text"}, path)
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

func TestRunGainTextAlignsTotalRowWithCountColumn(t *testing.T) {
	path := seedGainDB(t)
	out := captureStdout(t, func() error {
		return RunGain([]string{flagFormat, "text"}, path)
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

func TestRunGainTextTieBreakOrderingCountThenNativeThenTool(t *testing.T) {
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
		return RunGain([]string{flagFormat, "text"}, path)
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
	if !strings.Contains(periodText, "BUCKET") || !strings.Contains(periodText, "period=day") {
		t.Fatalf("unexpected gain period text: %q", periodText)
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
}

func TestRunHistoryRejectsPeriodFlag(t *testing.T) {
	path := seedGainDB(t)
	if err := RunHistory([]string{flagPeriod, "day"}, path); err == nil || !strings.Contains(err.Error(), "--period is only valid for gain") {
		t.Fatalf("expected history period rejection, got: %v", err)
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
