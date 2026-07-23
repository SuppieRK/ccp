package metrics

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go-command-compression-proxy/internal/projectfiles"

	bolt "go.etcd.io/bbolt"
)

const (
	writeTimeout          = 100 * time.Millisecond
	maxCommandTextRunes   = 1024
	defaultMissedTopLimit = 5
	dateFormatYMD         = "2006-01-02"
	defaultRetention      = 90 * 24 * time.Hour
	pruneBatchLimit       = 100
)

var (
	runsBucket   = []byte("runs")
	eventsBucket = []byte("event_ids")
)

type RunMetric struct {
	Timestamp             time.Time
	Command               string
	Tool                  string
	Dispatch              string
	RawBytes              int
	KeptBytes             int
	ExitCode              int
	DurationMS            int64
	Passthrough           bool
	FilterSourceKind      string
	FilterPath            string
	FilterHash            string
	RegistryBuildRecorded bool
	RegistryBuildMS       int64
	RegistrySources       []RegistrySourceBuildMetric
}

type Summary struct {
	Runs      int     `json:"runs"`
	RawLines  int     `json:"raw_lines"`
	KeptLines int     `json:"kept_lines"`
	Dropped   int     `json:"dropped_lines"`
	DropRatio float64 `json:"drop_ratio"`
}

type QueryOptions struct {
	Since  time.Duration
	Tool   string
	Failed bool
	Period string
}

type SummaryRow struct {
	Command               string  `json:"command"`
	Commands              int64   `json:"commands"`
	RawBytes              int64   `json:"raw_bytes"`
	KeptBytes             int64   `json:"kept_bytes"`
	DroppedBytes          int64   `json:"dropped_bytes"`
	DropRatio             float64 `json:"drop_ratio"`
	EstimatedInputTokens  int64   `json:"estimated_input_tokens"`
	EstimatedOutputTokens int64   `json:"estimated_output_tokens"`
	EstimatedSavedTokens  int64   `json:"estimated_saved_tokens"`
	EstimatedSavingsPct   float64 `json:"estimated_savings_pct"`
}

type SummaryToolRow struct {
	Tool                  string  `json:"tool"`
	Commands              int64   `json:"commands"`
	RawBytes              int64   `json:"raw_bytes"`
	KeptBytes             int64   `json:"kept_bytes"`
	DroppedBytes          int64   `json:"dropped_bytes"`
	DropRatio             float64 `json:"drop_ratio"`
	EstimatedInputTokens  int64   `json:"estimated_input_tokens"`
	EstimatedOutputTokens int64   `json:"estimated_output_tokens"`
	EstimatedSavedTokens  int64   `json:"estimated_saved_tokens"`
	EstimatedSavingsPct   float64 `json:"estimated_savings_pct"`
}

type SummaryTotal struct {
	Commands              int64   `json:"commands"`
	RawBytes              int64   `json:"raw_bytes"`
	KeptBytes             int64   `json:"kept_bytes"`
	DroppedBytes          int64   `json:"dropped_bytes"`
	DropRatio             float64 `json:"drop_ratio"`
	EstimatedInputTokens  int64   `json:"estimated_input_tokens"`
	EstimatedOutputTokens int64   `json:"estimated_output_tokens"`
	EstimatedSavedTokens  int64   `json:"estimated_saved_tokens"`
	EstimatedSavingsPct   float64 `json:"estimated_savings_pct"`
}

type MissedOpportunity struct {
	Command string `json:"command"`
	Count   int64  `json:"count"`
}

type HistoryRow struct {
	Timestamp             time.Time `json:"timestamp"`
	Command               string    `json:"command"`
	Tool                  string    `json:"tool"`
	DispatchKey           string    `json:"dispatch_key"`
	Filter                string    `json:"filter"`
	Case                  string    `json:"case"`
	FilterSourceKind      string    `json:"filter_source_kind"`
	FilterPath            string    `json:"filter_path"`
	FilterHash            string    `json:"filter_hash"`
	ExitCode              int       `json:"exit_code"`
	Failed                bool      `json:"failed"`
	Passthrough           bool      `json:"passthrough"`
	DurationMS            int64     `json:"duration_ms"`
	RawBytes              int64     `json:"raw_bytes"`
	KeptBytes             int64     `json:"kept_bytes"`
	DroppedBytes          int64     `json:"dropped_bytes"`
	DropRatio             float64   `json:"drop_ratio"`
	EstimatedInputTokens  int64     `json:"estimated_input_tokens"`
	EstimatedOutputTokens int64     `json:"estimated_output_tokens"`
	EstimatedSavedTokens  int64     `json:"estimated_saved_tokens"`
	EstimatedSavingsPct   float64   `json:"estimated_savings_pct"`
}

type PeriodRow struct {
	Bucket                string  `json:"bucket"`
	BucketStart           string  `json:"bucket_start"`
	BucketEnd             string  `json:"bucket_end"`
	Commands              int64   `json:"commands"`
	RawBytes              int64   `json:"raw_bytes"`
	KeptBytes             int64   `json:"kept_bytes"`
	DroppedBytes          int64   `json:"dropped_bytes"`
	DropRatio             float64 `json:"drop_ratio"`
	EstimatedInputTokens  int64   `json:"estimated_input_tokens"`
	EstimatedOutputTokens int64   `json:"estimated_output_tokens"`
	EstimatedSavedTokens  int64   `json:"estimated_saved_tokens"`
	EstimatedSavingsPct   float64 `json:"estimated_savings_pct"`
}

type PerformanceRow struct {
	Tool                  string  `json:"tool"`
	Filter                string  `json:"filter"`
	Case                  string  `json:"case"`
	DispatchKey           string  `json:"dispatch_key"`
	FilterSourceKind      string  `json:"filter_source_kind"`
	FilterPath            string  `json:"filter_path"`
	FilterHash            string  `json:"filter_hash"`
	Commands              int64   `json:"commands"`
	PassthroughCommands   int64   `json:"passthrough_commands"`
	PassthroughRate       float64 `json:"passthrough_rate"`
	FailedCommands        int64   `json:"failed_commands"`
	FailedRate            float64 `json:"failed_rate"`
	AvgDurationMS         float64 `json:"avg_duration_ms"`
	RawBytes              int64   `json:"raw_bytes"`
	KeptBytes             int64   `json:"kept_bytes"`
	DroppedBytes          int64   `json:"dropped_bytes"`
	DropRatio             float64 `json:"drop_ratio"`
	EstimatedInputTokens  int64   `json:"estimated_input_tokens"`
	EstimatedOutputTokens int64   `json:"estimated_output_tokens"`
	EstimatedSavedTokens  int64   `json:"estimated_saved_tokens"`
	EstimatedSavingsPct   float64 `json:"estimated_savings_pct"`
}

type RegistrySourceBuildMetric struct {
	SourceKind  string `json:"source_kind"`
	SourceDir   string `json:"source_dir"`
	Definitions int64  `json:"definitions"`
	Compiled    int64  `json:"compiled"`
	DurationMS  int64  `json:"duration_ms"`
	Error       string `json:"error"`
}

type RegistryBuildEvent struct {
	DurationMS int64                       `json:"duration_ms"`
	Sources    []RegistrySourceBuildMetric `json:"sources"`
}

type RegistryBuildSummary struct {
	Builds        int64   `json:"builds"`
	AvgDurationMS float64 `json:"avg_duration_ms"`
	P95DurationMS int64   `json:"p95_duration_ms"`
	MaxDurationMS int64   `json:"max_duration_ms"`
}

type RegistrySourceBuildRow struct {
	SourceKind    string  `json:"source_kind"`
	SourceDir     string  `json:"source_dir"`
	Builds        int64   `json:"builds"`
	Errors        int64   `json:"errors"`
	Definitions   int64   `json:"definitions"`
	Compiled      int64   `json:"compiled"`
	AvgDurationMS float64 `json:"avg_duration_ms"`
	P95DurationMS int64   `json:"p95_duration_ms"`
	MaxDurationMS int64   `json:"max_duration_ms"`
}

type runRecord struct {
	TimestampUnix         int64
	Command               string
	Tool                  string
	Dispatch              string
	RawBytes              int64
	KeptBytes             int64
	ExitCode              int
	DurationMS            int64
	Passthrough           bool
	FilterSourceKind      string
	FilterPath            string
	FilterHash            string
	RegistryBuildRecorded bool
	RegistryBuildMS       int64
	RegistrySources       []RegistrySourceBuildMetric
}

type periodAcc struct {
	bucket string
	start  string
	end    string
	raw    int64
	kept   int64
	count  int64
}

type performanceAcc struct {
	row      PerformanceRow
	duration int64
}

type registrySourceBuildAcc struct {
	row       RegistrySourceBuildRow
	durations []int64
}

type derivedMetricTargets struct {
	droppedBytes *int64
	dropRatio    *float64
	inputTokens  *int64
	outputTokens *int64
	savedTokens  *int64
	savingsPct   *float64
}

func Append(path string, metric RunMetric) (err error) {
	return appendMetric("", path, metric)
}

// AppendProject appends a runtime metric while requiring the database and all
// writable state to remain beneath projectRoot.
func AppendProject(projectRoot, path string, metric RunMetric) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := projectfiles.RejectSymlinkPath(path); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		if err := projectfiles.ValidateRegularFileBeneath(projectRoot, path); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	rec := normalizeMetric(metric)
	if err := spoolProjectMetric(projectRoot, path, rec); err != nil {
		return err
	}
	// A durable spool item is success for the foreground command. A locked or
	// temporarily unavailable database is retried by the next invocation.
	_ = consolidateProjectSpool(projectRoot, path)
	return nil
}

func appendMetric(projectRoot, path string, metric RunMetric) (err error) {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := ensureLocalCCPGitignore(projectRoot, path); err != nil {
		return err
	}
	if !fileExists(path) {
		if err := ensureSchema(projectRoot, path); err != nil {
			return err
		}
	}
	db, err := openDBAt(projectRoot, path, false)
	if err != nil {
		return err
	}
	defer func() {
		if db != nil {
			closeBoltDBWithErr(db, &err)
		}
	}()

	rec := normalizeMetric(metric)
	if err := writeRunRecord(db, rec); err == nil {
		return nil
	}
	if err := db.Close(); err != nil {
		return err
	}
	db = nil
	if err := ensureSchema(projectRoot, path); err != nil {
		return err
	}
	db, err = openDBAt(projectRoot, path, false)
	if err != nil {
		return err
	}
	return writeRunRecord(db, rec)
}

func normalizeMetric(metric RunMetric) runRecord {
	if metric.Timestamp.IsZero() {
		metric.Timestamp = time.Now().UTC()
	} else {
		metric.Timestamp = metric.Timestamp.UTC()
	}
	metric.Command = truncateCommand(metric.Command)
	if metric.Tool == "" {
		metric.Tool = "unknown"
	}
	return runRecord{
		TimestampUnix:         metric.Timestamp.Unix(),
		Command:               metric.Command,
		Tool:                  metric.Tool,
		Dispatch:              metric.Dispatch,
		RawBytes:              int64(max0(metric.RawBytes)),
		KeptBytes:             int64(max0(metric.KeptBytes)),
		ExitCode:              metric.ExitCode,
		DurationMS:            max0i64(metric.DurationMS),
		Passthrough:           metric.Passthrough,
		FilterSourceKind:      strings.TrimSpace(metric.FilterSourceKind),
		FilterPath:            strings.TrimSpace(metric.FilterPath),
		FilterHash:            strings.TrimSpace(metric.FilterHash),
		RegistryBuildRecorded: metric.RegistryBuildRecorded,
		RegistryBuildMS:       max0i64(metric.RegistryBuildMS),
		RegistrySources:       normalizeRegistrySources(metric.RegistrySources),
	}
}

func writeRunRecord(db *bolt.DB, rec runRecord) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(runsBucket)
		if b == nil {
			return errors.New("metrics bucket missing")
		}
		seq, err := b.NextSequence()
		if err != nil {
			return err
		}
		key := encodeRunKey(rec.TimestampUnix, seq)
		val := encodeRunRecord(rec)
		return b.Put(key, val)
	})
}

// PurgeBefore removes records strictly older than cutoff.
func PurgeBefore(path string, cutoff time.Time) (int, error) {
	return purgeBefore("", path, cutoff)
}

// PurgeProjectBefore applies PurgeBefore through the contained project opener.
func PurgeProjectBefore(projectRoot, path string, cutoff time.Time) (int, error) {
	return purgeBefore(projectRoot, path, cutoff)
}

func purgeBefore(projectRoot, path string, cutoff time.Time) (removed int, err error) {
	if strings.TrimSpace(path) == "" || !fileExists(path) {
		return 0, nil
	}
	db, err := openDBAt(projectRoot, path, false)
	if err != nil {
		return 0, err
	}
	defer closeBoltDBWithErr(db, &err)
	err = db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(runsBucket)
		if bucket == nil {
			return nil
		}
		cursor := bucket.Cursor()
		for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
			if len(key) < 8 || getBoundedInt64FromU64(key[:8]) >= cutoff.UTC().Unix() {
				break
			}
			if err := cursor.Delete(); err != nil {
				return err
			}
			removed++
		}
		return nil
	})
	return removed, err
}

func LoadSummary(path string) (Summary, error) {
	total, err := QuerySummary(path, QueryOptions{})
	if err != nil {
		return Summary{}, err
	}
	return Summary{
		Runs:      int(total.Commands),
		RawLines:  int(total.RawBytes),
		KeptLines: int(total.KeptBytes),
		Dropped:   int(total.DroppedBytes),
		DropRatio: total.DropRatio,
	}, nil
}

func QuerySummaryRows(path string, opts QueryOptions) (rows []SummaryRow, err error) {
	if strings.TrimSpace(path) == "" || !fileExists(path) {
		return nil, nil
	}
	db, err := openDB(path, true)
	if err != nil {
		return nil, err
	}
	defer closeBoltDBWithErr(db, &err)

	grouped := make(map[string]SummaryRow, 32)
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(runsBucket)
		if b == nil {
			return nil
		}
		threshold := sinceThreshold(opts)
		return b.ForEach(func(_, v []byte) error {
			rec := decodeRunRecord(v)
			if !matchesOptions(rec, opts, threshold) {
				return nil
			}
			row := grouped[rec.Command]
			row.Command = rec.Command
			row.Commands++
			row.RawBytes += rec.RawBytes
			row.KeptBytes += rec.KeptBytes
			grouped[rec.Command] = row
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	out := make([]SummaryRow, 0, len(grouped))
	for _, r := range grouped {
		fillSummaryDerived(&r)
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Commands != out[j].Commands {
			return out[i].Commands > out[j].Commands
		}
		return out[i].Command < out[j].Command
	})
	return out, nil
}

func QuerySummaryRowsByTool(path string, opts QueryOptions) (rows []SummaryToolRow, err error) {
	if strings.TrimSpace(path) == "" || !fileExists(path) {
		return nil, nil
	}
	db, err := openDB(path, true)
	if err != nil {
		return nil, err
	}
	defer closeBoltDBWithErr(db, &err)

	grouped := make(map[string]SummaryToolRow, 16)
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(runsBucket)
		if b == nil {
			return nil
		}
		threshold := sinceThreshold(opts)
		return b.ForEach(func(_, v []byte) error {
			rec := decodeRunRecord(v)
			if !matchesOptions(rec, opts, threshold) {
				return nil
			}
			key := rec.Tool
			if key == "" {
				key = "unknown"
			}
			row := grouped[key]
			row.Tool = key
			row.Commands++
			row.RawBytes += rec.RawBytes
			row.KeptBytes += rec.KeptBytes
			grouped[key] = row
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	out := make([]SummaryToolRow, 0, len(grouped))
	for _, r := range grouped {
		fillSummaryToolDerived(&r)
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Commands != out[j].Commands {
			return out[i].Commands > out[j].Commands
		}
		return out[i].Tool < out[j].Tool
	})
	return out, nil
}

func QuerySummary(path string, opts QueryOptions) (total SummaryTotal, err error) {
	if strings.TrimSpace(path) == "" || !fileExists(path) {
		return SummaryTotal{}, nil
	}
	db, err := openDB(path, true)
	if err != nil {
		return SummaryTotal{}, err
	}
	defer closeBoltDBWithErr(db, &err)

	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(runsBucket)
		if b == nil {
			return nil
		}
		threshold := sinceThreshold(opts)
		return b.ForEach(func(_, v []byte) error {
			rec := decodeRunRecord(v)
			if !matchesOptions(rec, opts, threshold) {
				return nil
			}
			total.Commands++
			total.RawBytes += rec.RawBytes
			total.KeptBytes += rec.KeptBytes
			return nil
		})
	})
	if err != nil {
		return SummaryTotal{}, err
	}
	fillTotalDerived(&total)
	return total, nil
}

func QueryMissedOpportunities(path string, opts QueryOptions, limit int) (opps []MissedOpportunity, err error) {
	if strings.TrimSpace(path) == "" || !fileExists(path) {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultMissedTopLimit
	}
	db, err := openDB(path, true)
	if err != nil {
		return nil, err
	}
	defer closeBoltDBWithErr(db, &err)

	grouped := make(map[string]int64, 16)
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(runsBucket)
		if b == nil {
			return nil
		}
		threshold := sinceThreshold(opts)
		return b.ForEach(func(_, v []byte) error {
			rec := decodeRunRecord(v)
			if !matchesOptions(rec, opts, threshold) || !rec.Passthrough {
				return nil
			}
			grouped[rec.Command]++
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	out := make([]MissedOpportunity, 0, len(grouped))
	for cmd, cnt := range grouped {
		out = append(out, MissedOpportunity{Command: cmd, Count: cnt})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Command < out[j].Command
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func QueryHistory(path string, opts QueryOptions) (history []HistoryRow, err error) {
	if strings.TrimSpace(path) == "" || !fileExists(path) {
		return nil, nil
	}
	db, err := openDB(path, true)
	if err != nil {
		return nil, err
	}
	defer closeBoltDBWithErr(db, &err)

	out := make([]HistoryRow, 0, 64)
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(runsBucket)
		if b == nil {
			return nil
		}
		threshold := sinceThreshold(opts)
		return b.ForEach(func(_, v []byte) error {
			rec := decodeRunRecord(v)
			if !matchesOptions(rec, opts, threshold) {
				return nil
			}
			out = append(out, historyRowFromRecord(rec))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	reverseHistoryRows(out)
	return out, nil
}

func QueryPeriod(path string, opts QueryOptions) (periodRows []PeriodRow, err error) {
	if strings.TrimSpace(path) == "" || !fileExists(path) {
		return nil, nil
	}
	db, err := openDB(path, true)
	if err != nil {
		return nil, err
	}
	defer closeBoltDBWithErr(db, &err)

	groups := make(map[string]*periodAcc, 16)
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(runsBucket)
		if b == nil {
			return nil
		}
		threshold := sinceThreshold(opts)
		return b.ForEach(func(_, v []byte) error {
			rec := decodeRunRecord(v)
			if !matchesOptions(rec, opts, threshold) {
				return nil
			}
			return updatePeriodAcc(groups, rec, opts.Period)
		})
	})
	if err != nil {
		return nil, err
	}

	out := make([]PeriodRow, 0, len(groups))
	for _, g := range groups {
		out = append(out, periodRowFromAcc(g))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].BucketStart > out[j].BucketStart
	})
	return out, nil
}

func QueryPerformanceRows(path string, opts QueryOptions) (rows []PerformanceRow, err error) {
	if strings.TrimSpace(path) == "" || !fileExists(path) {
		return nil, nil
	}
	db, err := openDB(path, true)
	if err != nil {
		return nil, err
	}
	defer closeBoltDBWithErr(db, &err)

	grouped := make(map[string]*performanceAcc, 32)
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(runsBucket)
		if b == nil {
			return nil
		}
		threshold := sinceThreshold(opts)
		return b.ForEach(func(_, v []byte) error {
			rec := decodeRunRecord(v)
			if !matchesOptions(rec, opts, threshold) {
				return nil
			}
			updatePerformanceAcc(grouped, rec)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	out := make([]PerformanceRow, 0, len(grouped))
	for _, acc := range grouped {
		row := acc.row
		fillPerformanceDerived(&row, acc.duration)
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Commands != out[j].Commands {
			return out[i].Commands > out[j].Commands
		}
		if out[i].EstimatedSavedTokens != out[j].EstimatedSavedTokens {
			return out[i].EstimatedSavedTokens > out[j].EstimatedSavedTokens
		}
		if out[i].Tool != out[j].Tool {
			return out[i].Tool < out[j].Tool
		}
		if out[i].Filter != out[j].Filter {
			return out[i].Filter < out[j].Filter
		}
		return out[i].Case < out[j].Case
	})
	return out, nil
}

func QueryRegistryBuild(path string, opts QueryOptions) (summary RegistryBuildSummary, sourceRows []RegistrySourceBuildRow, err error) {
	events, err := QueryRegistryBuildEvents(path, opts)
	if err != nil {
		return RegistryBuildSummary{}, nil, err
	}
	return RegistryBuildSummaryFromEvents(events), RegistrySourceBuildRowsFromEvents(events), nil
}

func QueryRegistryBuildEvents(path string, opts QueryOptions) (events []RegistryBuildEvent, err error) {
	if strings.TrimSpace(path) == "" || !fileExists(path) {
		return nil, nil
	}
	db, err := openDB(path, true)
	if err != nil {
		return nil, err
	}
	defer closeBoltDBWithErr(db, &err)

	out := make([]RegistryBuildEvent, 0, 16)
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(runsBucket)
		if b == nil {
			return nil
		}
		threshold := sinceThreshold(opts)
		return b.ForEach(func(_, v []byte) error {
			rec := decodeRunRecord(v)
			if !matchesOptions(rec, opts, threshold) || !rec.RegistryBuildRecorded {
				return nil
			}
			out = append(out, RegistryBuildEvent{
				DurationMS: rec.RegistryBuildMS,
				Sources:    append([]RegistrySourceBuildMetric(nil), rec.RegistrySources...),
			})
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func RegistryBuildSummaryFromEvents(events []RegistryBuildEvent) RegistryBuildSummary {
	durations := make([]int64, 0, len(events))
	for _, event := range events {
		durations = append(durations, event.DurationMS)
	}
	return registryBuildSummaryFromDurations(durations)
}

func RegistrySourceBuildRowsFromEvents(events []RegistryBuildEvent) []RegistrySourceBuildRow {
	groupedSources := make(map[string]*registrySourceBuildAcc, 8)
	for _, event := range events {
		for _, source := range event.Sources {
			updateRegistrySourceBuildAcc(groupedSources, source)
		}
	}
	out := make([]RegistrySourceBuildRow, 0, len(groupedSources))
	for _, acc := range groupedSources {
		row := acc.row
		fillRegistrySourceBuildDerived(&row, acc.durations)
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AvgDurationMS != out[j].AvgDurationMS {
			return out[i].AvgDurationMS > out[j].AvgDurationMS
		}
		if out[i].MaxDurationMS != out[j].MaxDurationMS {
			return out[i].MaxDurationMS > out[j].MaxDurationMS
		}
		if out[i].SourceKind != out[j].SourceKind {
			return out[i].SourceKind < out[j].SourceKind
		}
		return out[i].SourceDir < out[j].SourceDir
	})
	return out
}

func historyRowFromRecord(rec runRecord) HistoryRow {
	filterID, caseID := dispatchFilterCase(rec.Dispatch, rec.Tool)
	r := HistoryRow{
		Timestamp:        time.Unix(rec.TimestampUnix, 0).UTC(),
		Command:          rec.Command,
		Tool:             rec.Tool,
		DispatchKey:      rec.Dispatch,
		Filter:           filterID,
		Case:             caseID,
		FilterSourceKind: rec.FilterSourceKind,
		FilterPath:       rec.FilterPath,
		FilterHash:       rec.FilterHash,
		ExitCode:         rec.ExitCode,
		Failed:           rec.ExitCode != 0,
		Passthrough:      rec.Passthrough,
		DurationMS:       rec.DurationMS,
		RawBytes:         rec.RawBytes,
		KeptBytes:        rec.KeptBytes,
	}
	fillDerivedMetrics(r.RawBytes, r.KeptBytes, derivedMetricTargets{
		droppedBytes: &r.DroppedBytes,
		dropRatio:    &r.DropRatio,
		inputTokens:  &r.EstimatedInputTokens,
		outputTokens: &r.EstimatedOutputTokens,
		savedTokens:  &r.EstimatedSavedTokens,
		savingsPct:   &r.EstimatedSavingsPct,
	})
	return r
}

func updateRegistrySourceBuildAcc(grouped map[string]*registrySourceBuildAcc, source RegistrySourceBuildMetric) {
	key := strings.Join([]string{source.SourceKind, source.SourceDir}, "\x00")
	acc := grouped[key]
	if acc == nil {
		acc = &registrySourceBuildAcc{
			row: RegistrySourceBuildRow{
				SourceKind: source.SourceKind,
				SourceDir:  source.SourceDir,
			},
		}
		grouped[key] = acc
	}
	acc.row.Builds++
	if strings.TrimSpace(source.Error) != "" {
		acc.row.Errors++
	}
	acc.row.Definitions += max0i64(source.Definitions)
	acc.row.Compiled += max0i64(source.Compiled)
	acc.durations = append(acc.durations, max0i64(source.DurationMS))
}

func registryBuildSummaryFromDurations(durations []int64) RegistryBuildSummary {
	if len(durations) == 0 {
		return RegistryBuildSummary{}
	}
	var sum int64
	var maxDuration int64
	for _, duration := range durations {
		duration = max0i64(duration)
		sum += duration
		maxDuration = max(maxDuration, duration)
	}
	return RegistryBuildSummary{
		Builds:        int64(len(durations)),
		AvgDurationMS: float64(sum) / float64(len(durations)),
		P95DurationMS: percentileDuration(durations, 0.95),
		MaxDurationMS: maxDuration,
	}
}

func fillRegistrySourceBuildDerived(row *RegistrySourceBuildRow, durations []int64) {
	summary := registryBuildSummaryFromDurations(durations)
	row.Builds = summary.Builds
	row.AvgDurationMS = summary.AvgDurationMS
	row.P95DurationMS = summary.P95DurationMS
	row.MaxDurationMS = summary.MaxDurationMS
}

func percentileDuration(durations []int64, percentile float64) int64 {
	if len(durations) == 0 {
		return 0
	}
	sorted := append([]int64(nil), durations...)
	for i, duration := range sorted {
		sorted[i] = max0i64(duration)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})
	index := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	index = max(0, min(index, len(sorted)-1))
	return sorted[index]
}

func updatePerformanceAcc(grouped map[string]*performanceAcc, rec runRecord) {
	filterID, caseID := dispatchFilterCase(rec.Dispatch, rec.Tool)
	key := strings.Join([]string{
		rec.Tool,
		filterID,
		caseID,
		rec.Dispatch,
		rec.FilterSourceKind,
		rec.FilterPath,
		rec.FilterHash,
	}, "\x00")
	acc := grouped[key]
	if acc == nil {
		acc = &performanceAcc{
			row: PerformanceRow{
				Tool:             rec.Tool,
				Filter:           filterID,
				Case:             caseID,
				DispatchKey:      rec.Dispatch,
				FilterSourceKind: rec.FilterSourceKind,
				FilterPath:       rec.FilterPath,
				FilterHash:       rec.FilterHash,
			},
		}
		grouped[key] = acc
	}
	acc.row.Commands++
	if rec.Passthrough {
		acc.row.PassthroughCommands++
	}
	if rec.ExitCode != 0 {
		acc.row.FailedCommands++
	}
	acc.row.RawBytes += rec.RawBytes
	acc.row.KeptBytes += rec.KeptBytes
	acc.duration += rec.DurationMS
}

func dispatchFilterCase(dispatch, fallbackTool string) (string, string) {
	dispatch = strings.TrimSpace(dispatch)
	fallbackTool = strings.TrimSpace(fallbackTool)
	if dispatch == "" {
		if fallbackTool == "" {
			return "unknown", ""
		}
		return fallbackTool, ""
	}
	filterID, caseID, ok := strings.Cut(dispatch, "|")
	filterID = strings.TrimSpace(filterID)
	caseID = strings.TrimSpace(caseID)
	if filterID == "" {
		filterID = fallbackTool
	}
	if filterID == "" {
		filterID = "unknown"
	}
	if !ok {
		return filterID, ""
	}
	return filterID, caseID
}

func reverseHistoryRows(rows []HistoryRow) {

	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
}

func updatePeriodAcc(groups map[string]*periodAcc, rec runRecord, period string) error {
	bucket, start, end, err := bucketFor(time.Unix(rec.TimestampUnix, 0).UTC(), period)
	if err != nil {
		return err
	}
	acc := groups[bucket]
	if acc == nil {
		acc = &periodAcc{bucket: bucket, start: start, end: end}
		groups[bucket] = acc
	}
	acc.count++
	acc.raw += rec.RawBytes
	acc.kept += rec.KeptBytes
	return nil
}

func periodRowFromAcc(acc *periodAcc) PeriodRow {
	r := PeriodRow{
		Bucket:      acc.bucket,
		BucketStart: acc.start,
		BucketEnd:   acc.end,
		Commands:    acc.count,
		RawBytes:    acc.raw,
		KeptBytes:   acc.kept,
	}
	fillDerivedMetrics(r.RawBytes, r.KeptBytes, derivedMetricTargets{
		droppedBytes: &r.DroppedBytes,
		dropRatio:    &r.DropRatio,
		inputTokens:  &r.EstimatedInputTokens,
		outputTokens: &r.EstimatedOutputTokens,
		savedTokens:  &r.EstimatedSavedTokens,
		savingsPct:   &r.EstimatedSavingsPct,
	})
	return r
}

func openDB(path string, readOnly bool) (*bolt.DB, error) {
	return openDBAt("", path, readOnly)
}

func openDBAt(projectRoot, path string, readOnly bool) (*bolt.DB, error) {
	opts := &bolt.Options{
		ReadOnly:       readOnly,
		Timeout:        writeTimeout,
		NoFreelistSync: true,
	}
	if strings.TrimSpace(projectRoot) != "" {
		expectedPath := filepath.Clean(path)
		opts.OpenFile = func(name string, flag int, mode os.FileMode) (*os.File, error) {
			if filepath.Clean(name) != expectedPath {
				return nil, fmt.Errorf("refuse unexpected metrics database path %q", name)
			}
			return projectfiles.OpenFileBeneath(projectRoot, name, flag, mode)
		}
	}
	db, err := bolt.Open(path, 0o600, opts)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func encodeRunKey(tsUnix int64, seq uint64) []byte {
	key := make([]byte, 16)
	putNonNegativeInt64AsU64(key[0:8], tsUnix)
	putU64(key[8:16], seq)
	return key
}

func putU64(dst []byte, v uint64) {
	binary.BigEndian.PutUint64(dst, v)
}

func getU64(src []byte) uint64 {
	return binary.BigEndian.Uint64(src)
}

func encodeRunRecord(rec runRecord) []byte {
	cmd := []byte(rec.Command)
	tool := []byte(rec.Tool)
	dispatch := []byte(rec.Dispatch)
	sourceKind := []byte(rec.FilterSourceKind)
	filterPath := []byte(rec.FilterPath)
	filterHash := []byte(rec.FilterHash)
	registrySources := []byte("")
	if rec.RegistryBuildRecorded {
		registrySources = []byte(encodeRegistrySources(rec.RegistrySources))
	}
	sz := 8 + 4 + len(cmd) + 4 + len(tool) + 4 + len(dispatch) + 8 + 8 + 8 + 8 + 1 +
		4 + len(sourceKind) + 4 + len(filterPath) + 4 + len(filterHash)
	if rec.RegistryBuildRecorded {
		sz += 8 + 4 + len(registrySources)
	}
	out := make([]byte, sz)
	i := 0
	putNonNegativeInt64AsU64(out[i:i+8], rec.TimestampUnix)
	i += 8
	i = putEncodedString(out, i, cmd)
	i = putEncodedString(out, i, tool)
	i = putEncodedString(out, i, dispatch)
	putNonNegativeInt64AsU64(out[i:i+8], rec.RawBytes)
	i += 8
	putNonNegativeInt64AsU64(out[i:i+8], rec.KeptBytes)
	i += 8
	putIntAsU64(out[i:i+8], rec.ExitCode)
	i += 8
	putNonNegativeInt64AsU64(out[i:i+8], rec.DurationMS)
	i += 8
	if rec.Passthrough {
		out[i] = 1
	}
	i++
	i = putEncodedString(out, i, sourceKind)
	i = putEncodedString(out, i, filterPath)
	i = putEncodedString(out, i, filterHash)
	if rec.RegistryBuildRecorded {
		putNonNegativeInt64AsU64(out[i:i+8], rec.RegistryBuildMS)
		i += 8
		putEncodedString(out, i, registrySources)
	}
	return out
}

func decodeRunRecord(b []byte) runRecord {

	if len(b) < 8+4+4+4+8+8+8+8+1 {
		return runRecord{}
	}
	i := 0
	rec := runRecord{}
	rec.TimestampUnix = getBoundedInt64FromU64(b[i : i+8])
	i += 8
	cmdLen := getBoundedIntFromU32(b[i : i+4])
	i += 4
	if i+cmdLen > len(b) {
		return runRecord{}
	}
	rec.Command = string(b[i : i+cmdLen])
	i += cmdLen
	toolLen := getBoundedIntFromU32(b[i : i+4])
	i += 4
	if i+toolLen > len(b) {
		return runRecord{}
	}
	rec.Tool = string(b[i : i+toolLen])
	i += toolLen
	dispatchLen := getBoundedIntFromU32(b[i : i+4])
	i += 4
	if i+dispatchLen > len(b) {
		return runRecord{}
	}
	rec.Dispatch = string(b[i : i+dispatchLen])
	i += dispatchLen
	if i+8*4+1 > len(b) {
		return runRecord{}
	}
	rec.RawBytes = getBoundedInt64FromU64(b[i : i+8])
	i += 8
	rec.KeptBytes = getBoundedInt64FromU64(b[i : i+8])
	i += 8
	rec.ExitCode = getBoundedSignedIntFromU64(b[i : i+8])
	i += 8
	rec.DurationMS = getBoundedInt64FromU64(b[i : i+8])
	i += 8
	rec.Passthrough = b[i] == 1
	i++
	if rec.Tool == "" {
		rec.Tool = "unknown"
	}
	if i == len(b) {
		return rec
	}
	var ok bool
	rec.FilterSourceKind, i, ok = getEncodedString(b, i)
	if !ok {
		return runRecord{}
	}
	rec.FilterPath, i, ok = getEncodedString(b, i)
	if !ok {
		return runRecord{}
	}
	rec.FilterHash, i, ok = getEncodedString(b, i)
	if !ok {
		return runRecord{}
	}
	if i == len(b) {
		return rec
	}
	if i+8 > len(b) {
		return runRecord{}
	}
	rec.RegistryBuildRecorded = true
	rec.RegistryBuildMS = getBoundedInt64FromU64(b[i : i+8])
	i += 8
	rawSources, _, ok := getEncodedString(b, i)
	if !ok {
		return runRecord{}
	}
	rec.RegistrySources = decodeRegistrySources(rawSources)
	return rec
}

func normalizeRegistrySources(sources []RegistrySourceBuildMetric) []RegistrySourceBuildMetric {
	out := make([]RegistrySourceBuildMetric, 0, len(sources))
	for _, source := range sources {
		out = append(out, RegistrySourceBuildMetric{
			SourceKind:  strings.TrimSpace(source.SourceKind),
			SourceDir:   strings.TrimSpace(source.SourceDir),
			Definitions: max0i64(source.Definitions),
			Compiled:    max0i64(source.Compiled),
			DurationMS:  max0i64(source.DurationMS),
			Error:       strings.TrimSpace(source.Error),
		})
	}
	return out
}

func encodeRegistrySources(sources []RegistrySourceBuildMetric) string {
	if len(sources) == 0 {
		return ""
	}
	raw, err := json.Marshal(normalizeRegistrySources(sources))
	if err != nil {
		return ""
	}
	return string(raw)
}

func decodeRegistrySources(raw string) []RegistrySourceBuildMetric {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []RegistrySourceBuildMetric
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return normalizeRegistrySources(out)
}

func putEncodedString(dst []byte, offset int, value []byte) int {
	putLengthU32(dst[offset:offset+4], len(value))
	offset += 4
	copy(dst[offset:offset+len(value)], value)
	return offset + len(value)
}

func getEncodedString(src []byte, offset int) (string, int, bool) {
	if offset+4 > len(src) {
		return "", offset, false
	}
	valueLen := getBoundedIntFromU32(src[offset : offset+4])
	offset += 4
	if offset+valueLen > len(src) {
		return "", offset, false
	}
	return string(src[offset : offset+valueLen]), offset + valueLen, true
}

func putU32(dst []byte, v uint32) {
	binary.BigEndian.PutUint32(dst, v)
}

func getU32(src []byte) uint32 {
	return binary.BigEndian.Uint32(src)
}

func putNonNegativeInt64AsU64(dst []byte, v int64) {
	if v < 0 {
		v = 0
	}
	putU64(dst, uint64(v))
}

func putIntAsU64(dst []byte, v int) {
	putU64(dst, uint64(int64(v)))
}

func putLengthU32(dst []byte, n int) {
	switch {
	case n < 0:
		putU32(dst, 0)
	case n > math.MaxUint32:
		putU32(dst, math.MaxUint32)
	default:
		putU32(dst, uint32(n))
	}
}

func getBoundedInt64FromU64(src []byte) int64 {
	v := getU64(src)
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

func getBoundedSignedIntFromU64(src []byte) int {
	v := int64(getU64(src))
	switch {
	case v > int64(math.MaxInt):
		return math.MaxInt
	case v < int64(math.MinInt):
		return math.MinInt
	default:
		return int(v)
	}
}

func getBoundedIntFromU32(src []byte) int {
	v := getU32(src)
	if uint64(v) > uint64(math.MaxInt) {
		return math.MaxInt
	}
	return int(v)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sinceThreshold(opts QueryOptions) int64 {
	if opts.Since <= 0 {
		return 0
	}
	return time.Now().Add(-opts.Since).Unix()
}

func matchesOptions(rec runRecord, opts QueryOptions, sinceUnix int64) bool {
	if sinceUnix > 0 && rec.TimestampUnix < sinceUnix {
		return false
	}
	if opts.Tool != "" && rec.Tool != opts.Tool {
		return false
	}
	if opts.Failed && rec.ExitCode == 0 {
		return false
	}
	return true
}

func bucketFor(ts time.Time, period string) (bucket, start, end string, err error) {
	utc := ts.UTC()
	switch period {
	case "day":
		d := utc.Format(dateFormatYMD)
		return d, d, d, nil
	case "week":
		y, w := utc.ISOWeek()
		monday := startOfISOWeek(utc)
		sunday := monday.AddDate(0, 0, 6)
		return formatWeek(y, w), monday.Format(dateFormatYMD), sunday.Format(dateFormatYMD), nil
	case "month":
		startTime := time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
		endTime := startTime.AddDate(0, 1, -1)
		return startTime.Format("2006-01"), startTime.Format(dateFormatYMD), endTime.Format(dateFormatYMD), nil
	default:
		return "", "", "", errors.New("invalid period \"" + period + "\"")
	}
}

func formatWeek(year, week int) string {
	return fmt.Sprintf("%d-W%02d", year, week)
}

func startOfISOWeek(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(weekday - 1))
}

func fillSummaryDerived(r *SummaryRow) {
	fillDerivedMetrics(r.RawBytes, r.KeptBytes, derivedMetricTargets{
		droppedBytes: &r.DroppedBytes,
		dropRatio:    &r.DropRatio,
		inputTokens:  &r.EstimatedInputTokens,
		outputTokens: &r.EstimatedOutputTokens,
		savedTokens:  &r.EstimatedSavedTokens,
		savingsPct:   &r.EstimatedSavingsPct,
	})
}

func FillSummaryDerived(r *SummaryRow) {
	fillSummaryDerived(r)
}

func fillSummaryToolDerived(r *SummaryToolRow) {
	fillDerivedMetrics(r.RawBytes, r.KeptBytes, derivedMetricTargets{
		droppedBytes: &r.DroppedBytes,
		dropRatio:    &r.DropRatio,
		inputTokens:  &r.EstimatedInputTokens,
		outputTokens: &r.EstimatedOutputTokens,
		savedTokens:  &r.EstimatedSavedTokens,
		savingsPct:   &r.EstimatedSavingsPct,
	})
}

func FillSummaryToolDerived(r *SummaryToolRow) {
	fillSummaryToolDerived(r)
}

func fillTotalDerived(total *SummaryTotal) {
	fillDerivedMetrics(total.RawBytes, total.KeptBytes, derivedMetricTargets{
		droppedBytes: &total.DroppedBytes,
		dropRatio:    &total.DropRatio,
		inputTokens:  &total.EstimatedInputTokens,
		outputTokens: &total.EstimatedOutputTokens,
		savedTokens:  &total.EstimatedSavedTokens,
		savingsPct:   &total.EstimatedSavingsPct,
	})
}

func FillTotalDerived(total *SummaryTotal) {
	fillTotalDerived(total)
}

func FillPeriodDerived(r *PeriodRow) {
	fillDerivedMetrics(r.RawBytes, r.KeptBytes, derivedMetricTargets{
		droppedBytes: &r.DroppedBytes,
		dropRatio:    &r.DropRatio,
		inputTokens:  &r.EstimatedInputTokens,
		outputTokens: &r.EstimatedOutputTokens,
		savedTokens:  &r.EstimatedSavedTokens,
		savingsPct:   &r.EstimatedSavingsPct,
	})
}

func fillPerformanceDerived(row *PerformanceRow, durationMS int64) {
	fillDerivedMetrics(row.RawBytes, row.KeptBytes, derivedMetricTargets{
		droppedBytes: &row.DroppedBytes,
		dropRatio:    &row.DropRatio,
		inputTokens:  &row.EstimatedInputTokens,
		outputTokens: &row.EstimatedOutputTokens,
		savedTokens:  &row.EstimatedSavedTokens,
		savingsPct:   &row.EstimatedSavingsPct,
	})
	if row.Commands <= 0 {
		return
	}
	row.PassthroughRate = float64(row.PassthroughCommands) / float64(row.Commands)
	row.FailedRate = float64(row.FailedCommands) / float64(row.Commands)
	row.AvgDurationMS = float64(durationMS) / float64(row.Commands)
}

func FillPerformanceDerived(row *PerformanceRow, durationMS int64) {
	fillPerformanceDerived(row, durationMS)
}

func fillDerivedMetrics(rawBytes, keptBytes int64, targets derivedMetricTargets) {
	effectiveKeptBytes := max(int64(0), min(rawBytes, keptBytes))
	input := tokensFromBytes(rawBytes)
	output := tokensFromBytes(effectiveKeptBytes)
	if targets.droppedBytes != nil {
		*targets.droppedBytes = rawBytes - effectiveKeptBytes
	}
	if targets.dropRatio != nil {
		*targets.dropRatio = 0
		if rawBytes > 0 {
			*targets.dropRatio = float64(rawBytes-effectiveKeptBytes) / float64(rawBytes)
		}
	}
	if targets.inputTokens != nil {
		*targets.inputTokens = input
	}
	if targets.outputTokens != nil {
		*targets.outputTokens = output
	}
	if targets.savedTokens != nil {
		*targets.savedTokens = input - output
	}
	if targets.savingsPct != nil {
		*targets.savingsPct = 0
		if input > 0 {
			*targets.savingsPct = (float64(input-output) / float64(input)) * 100
		}
	}
}

func tokensFromBytes(v int64) int64 {
	if v <= 0 {
		return 0
	}
	return (v + 3) / 4
}

func TokensFromBytes(v int64) int64 {
	return tokensFromBytes(v)
}

func truncateCommand(cmd string) string {
	runes := []rune(cmd)
	if len(runes) <= maxCommandTextRunes {
		return cmd
	}
	return string(runes[:maxCommandTextRunes-3]) + "..."
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func max0i64(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func ensureSchema(projectRoot, path string) (err error) {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := ensureLocalCCPGitignore(projectRoot, path); err != nil {
		return err
	}
	if strings.TrimSpace(projectRoot) == "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
	}
	db, err := openDBAt(projectRoot, path, false)
	if err != nil {
		return err
	}
	defer closeBoltDBWithErr(db, &err)
	return db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(runsBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(eventsBucket)
		return err
	})
}

func closeBoltDBWithErr(db *bolt.DB, retErr *error) {
	if closeErr := db.Close(); *retErr == nil && closeErr != nil {
		*retErr = closeErr
	}
}

func ensureLocalCCPGitignore(projectRoot, path string) error {
	cleanPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil
	}
	if filepath.Base(cleanPath) != "gain.db" {
		return nil
	}
	ccpDir := filepath.Dir(cleanPath)
	if filepath.Base(ccpDir) != ".ccp" {
		return nil
	}
	pathProjectRoot := filepath.Dir(ccpDir)
	if strings.TrimSpace(projectRoot) == "" {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return nil
		}
		cwd, cwdErr = canonicalExistingPath(cwd)
		if cwdErr != nil {
			return nil
		}
		canonicalProjectRoot, canonicalErr := canonicalExistingPath(pathProjectRoot)
		if canonicalErr != nil || cwd != canonicalProjectRoot {
			return nil
		}
		projectRoot = pathProjectRoot
	} else {
		canonicalPath, containedErr := projectfiles.CanonicalPathBeneath(projectRoot, cleanPath)
		if containedErr != nil {
			return containedErr
		}
		if filepath.Clean(canonicalPath) != filepath.Clean(cleanPath) {
			return fmt.Errorf("metrics path %q does not use the canonical project path", path)
		}
		canonicalProjectRoot, canonicalErr := canonicalExistingPath(projectRoot)
		if canonicalErr != nil || filepath.Clean(canonicalProjectRoot) != filepath.Clean(pathProjectRoot) {
			return fmt.Errorf("metrics path %q is not the project .ccp database", path)
		}
		projectRoot = canonicalProjectRoot
	}
	gitMeta := filepath.Join(pathProjectRoot, ".git")
	info, err := os.Stat(gitMeta)
	if err != nil || !info.IsDir() {
		return nil
	}

	return projectfiles.EnsureNestedCCPGitignore(projectRoot)
}

func canonicalExistingPath(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func Bootstrap(path string) error {
	return ensureSchema("", path)
}

func IsTimeoutOrBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "busy") || strings.Contains(msg, "database is locked")
}
