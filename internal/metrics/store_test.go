package metrics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	metricsGoTestCommand = "go test ./..."
	errMkdirGitFmt       = "mkdir .git: %v"
	gitignoreFileName    = ".gitignore"
	gainDBFileName       = "gain.db"
	errAppendMetricFmt   = "append metric: %v"
)

func TestAppendAndLoadSummary(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "metrics", "runs.db")
	if err := Append(path, RunMetric{
		Tool:      "go",
		Command:   metricsGoTestCommand,
		RawBytes:  10,
		KeptBytes: 4,
		ExitCode:  0,
	}); err != nil {
		t.Fatalf("append first metric: %v", err)
	}
	if err := Append(path, RunMetric{
		Tool:      "git",
		Command:   "git status",
		RawBytes:  6,
		KeptBytes: 3,
		ExitCode:  1,
	}); err != nil {
		t.Fatalf("append second metric: %v", err)
	}

	got, err := LoadSummary(path)
	if err != nil {
		t.Fatalf("load summary: %v", err)
	}
	if got.Runs != 2 {
		t.Fatalf("runs = %d, want 2", got.Runs)
	}
	// Legacy compatibility fields are now byte-backed.
	if got.RawLines != 16 {
		t.Fatalf("raw aggregate = %d, want 16", got.RawLines)
	}
	if got.KeptLines != 7 {
		t.Fatalf("kept aggregate = %d, want 7", got.KeptLines)
	}
	if got.Dropped != 9 {
		t.Fatalf("dropped aggregate = %d, want 9", got.Dropped)
	}
	if got.DropRatio != 9.0/16.0 {
		t.Fatalf("drop ratio = %f, want %f", got.DropRatio, 9.0/16.0)
	}
}

func TestLoadSummaryReturnsZeroSummaryForMissingAndEmptyPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		path string
	}{
		{name: "missing-file", path: filepath.Join(t.TempDir(), "missing.db")},
		{name: "empty-path", path: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := LoadSummary(tc.path)
			if err != nil {
				t.Fatalf("load summary on %s: %v", tc.name, err)
			}
			if got != (Summary{}) {
				t.Fatalf("expected zero summary for %s, got %#v", tc.name, got)
			}
		})
	}
}

func TestAppendWithEmptyPathNoops(t *testing.T) {
	t.Parallel()
	if err := Append("", RunMetric{Tool: "noop", RawBytes: 1, KeptBytes: 1}); err != nil {
		t.Fatalf("append with empty path should noop, got error: %v", err)
	}
}

func TestAppendFailsWhenParentPathIsAFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	parentFile := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(parentFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write parent file: %v", err)
	}
	path := filepath.Join(parentFile, "metrics.db")
	err := Append(path, RunMetric{Tool: "go", RawBytes: 1, KeptBytes: 1})
	if err == nil {
		t.Fatalf("expected append to fail when parent path is a file")
	}
}

func TestAppendFailsWhenTargetPathIsDirectory(t *testing.T) {
	t.Parallel()
	path := t.TempDir()
	err := Append(path, RunMetric{Tool: "go", RawBytes: 1, KeptBytes: 1})
	if err == nil {
		t.Fatalf("expected append to fail when target path is a directory")
	}
}

func TestAppendTruncatesLongCommandText(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "metrics.db")
	long := strings.Repeat("x", 2000)
	if err := Append(path, RunMetric{
		Tool:      "go",
		Command:   long,
		RawBytes:  100,
		KeptBytes: 25,
		ExitCode:  0,
	}); err != nil {
		t.Fatalf("append long command: %v", err)
	}
	history, err := QueryHistory(path, QueryOptions{})
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history rows = %d, want 1", len(history))
	}
	if len([]rune(history[0].Command)) != 1024 {
		t.Fatalf("command length = %d, want 1024", len([]rune(history[0].Command)))
	}
	if !strings.HasSuffix(history[0].Command, "...") {
		t.Fatalf("expected deterministic truncation suffix, got %q", history[0].Command[len(history[0].Command)-6:])
	}
}

func TestAppendPreservesExplicitToolAndNormalizesBlankToolToUnknown(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "metrics.db")
	if err := Append(path, RunMetric{
		Tool:        "git",
		Command:     "git ls-files --stage",
		RawBytes:    10,
		KeptBytes:   10,
		Passthrough: true,
	}); err != nil {
		t.Fatalf("append explicit tool metric: %v", err)
	}
	if err := Append(path, RunMetric{
		Command:     "echo a && echo b",
		RawBytes:    4,
		KeptBytes:   4,
		Passthrough: true,
	}); err != nil {
		t.Fatalf("append blank tool metric: %v", err)
	}

	history, err := QueryHistory(path, QueryOptions{})
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history rows = %d, want 2", len(history))
	}
	if history[0].Tool != "unknown" {
		t.Fatalf("expected most recent blank tool metric to normalize to unknown, got %+v", history[0])
	}
	if history[1].Tool != "git" || !history[1].Passthrough {
		t.Fatalf("expected explicit canonical passthrough tool to be preserved, got %+v", history[1])
	}
}

func initGitProjectForMetrics(t *testing.T, gitignoreContent string) string {
	t.Helper()
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatalf(errMkdirGitFmt, err)
	}
	if gitignoreContent != "" {
		if err := os.WriteFile(filepath.Join(project, gitignoreFileName), []byte(gitignoreContent), 0o644); err != nil {
			t.Fatalf("write .gitignore: %v", err)
		}
	}
	return project
}

func TestAppendAddsDotCCPToGitignoreForLocalMetricsDB(t *testing.T) {
	project := initGitProjectForMetrics(t, "node_modules/\n")

	path := filepath.Join(project, ".ccp", gainDBFileName)
	if err := Append(path, RunMetric{Tool: "go", Command: metricsGoTestCommand, RawBytes: 16, KeptBytes: 8}); err != nil {
		t.Fatalf(errAppendMetricFmt, err)
	}

	b, err := os.ReadFile(filepath.Join(project, gitignoreFileName))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if got := string(b); got != "node_modules/\n.ccp\n" {
		t.Fatalf("unexpected .gitignore content: %q", got)
	}
}

func TestAppendDoesNotDuplicateDotCCPInGitignoreForLocalMetricsDB(t *testing.T) {
	project := initGitProjectForMetrics(t, ".ccp\n")

	path := filepath.Join(project, ".ccp", gainDBFileName)
	if err := Append(path, RunMetric{Tool: "go", Command: metricsGoTestCommand, RawBytes: 10, KeptBytes: 6}); err != nil {
		t.Fatalf(errAppendMetricFmt, err)
	}

	b, err := os.ReadFile(filepath.Join(project, gitignoreFileName))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if got := string(b); got != ".ccp\n" {
		t.Fatalf("expected single .ccp line, got: %q", got)
	}
}

func TestAppendSkipsGitignoreWhenMissingForLocalMetricsDB(t *testing.T) {
	project := initGitProjectForMetrics(t, "")

	path := filepath.Join(project, ".ccp", gainDBFileName)
	if err := Append(path, RunMetric{Tool: "go", Command: metricsGoTestCommand, RawBytes: 5, KeptBytes: 2}); err != nil {
		t.Fatalf(errAppendMetricFmt, err)
	}

	if _, err := os.Stat(filepath.Join(project, gitignoreFileName)); !os.IsNotExist(err) {
		t.Fatalf("expected .gitignore to remain absent, err=%v", err)
	}
}
