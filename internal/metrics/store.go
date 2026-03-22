package metrics

import (
	"encoding/binary"
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
)

var (
	runsBucket = []byte("runs")
)

type RunMetric struct {
	Timestamp   time.Time
	Command     string
	Tool        string
	Dispatch    string
	RawBytes    int
	KeptBytes   int
	ExitCode    int
	DurationMS  int64
	Passthrough bool
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

type runRecord struct {
	TimestampUnix int64
	Command       string
	Tool          string
	Dispatch      string
	RawBytes      int64
	KeptBytes     int64
	ExitCode      int
	DurationMS    int64
	Passthrough   bool
}

type periodAcc struct {
	bucket string
	start  string
	end    string
	raw    int64
	kept   int64
	count  int64
}

func Append(path string, metric RunMetric) (err error) {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if !fileExists(path) {
		if err := ensureSchema(path); err != nil {
			return err
		}
	}
	db, err := openDB(path, false)
	if err != nil {
		return err
	}
	defer closeBoltDBWithErr(db, &err)

	rec := normalizeMetric(metric)
	if writeRunRecord(db, rec) == nil {
		return nil
	}
	if err := ensureSchema(path); err != nil {
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
		TimestampUnix: metric.Timestamp.Unix(),
		Command:       metric.Command,
		Tool:          metric.Tool,
		Dispatch:      metric.Dispatch,
		RawBytes:      int64(max0(metric.RawBytes)),
		KeptBytes:     int64(max0(metric.KeptBytes)),
		ExitCode:      metric.ExitCode,
		DurationMS:    max0i64(metric.DurationMS),
		Passthrough:   metric.Passthrough,
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

func historyRowFromRecord(rec runRecord) HistoryRow {
	r := HistoryRow{
		Timestamp:   time.Unix(rec.TimestampUnix, 0).UTC(),
		Command:     rec.Command,
		Tool:        rec.Tool,
		DispatchKey: rec.Dispatch,
		ExitCode:    rec.ExitCode,
		Failed:      rec.ExitCode != 0,
		Passthrough: rec.Passthrough,
		DurationMS:  rec.DurationMS,
		RawBytes:    rec.RawBytes,
		KeptBytes:   rec.KeptBytes,
	}
	r.DroppedBytes = r.RawBytes - r.KeptBytes
	if r.RawBytes > 0 {
		r.DropRatio = float64(r.DroppedBytes) / float64(r.RawBytes)
	}
	r.EstimatedInputTokens = tokensFromBytes(r.RawBytes)
	r.EstimatedOutputTokens = tokensFromBytes(r.KeptBytes)
	r.EstimatedSavedTokens = r.EstimatedInputTokens - r.EstimatedOutputTokens
	if r.EstimatedInputTokens > 0 {
		r.EstimatedSavingsPct = (float64(r.EstimatedSavedTokens) / float64(r.EstimatedInputTokens)) * 100
	}
	return r
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
	r.DroppedBytes = r.RawBytes - r.KeptBytes
	if r.RawBytes > 0 {
		r.DropRatio = float64(r.DroppedBytes) / float64(r.RawBytes)
	}
	r.EstimatedInputTokens = tokensFromBytes(r.RawBytes)
	r.EstimatedOutputTokens = tokensFromBytes(r.KeptBytes)
	r.EstimatedSavedTokens = r.EstimatedInputTokens - r.EstimatedOutputTokens
	if r.EstimatedInputTokens > 0 {
		r.EstimatedSavingsPct = (float64(r.EstimatedSavedTokens) / float64(r.EstimatedInputTokens)) * 100
	}
	return r
}

func openDB(path string, readOnly bool) (*bolt.DB, error) {
	opts := &bolt.Options{
		ReadOnly:       readOnly,
		Timeout:        writeTimeout,
		NoFreelistSync: true,
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
	sz := 8 + 4 + len(cmd) + 4 + len(tool) + 4 + len(dispatch) + 8 + 8 + 8 + 8 + 1
	out := make([]byte, sz)
	i := 0
	putNonNegativeInt64AsU64(out[i:i+8], rec.TimestampUnix)
	i += 8
	putLengthU32(out[i:i+4], len(cmd))
	i += 4
	copy(out[i:i+len(cmd)], cmd)
	i += len(cmd)
	putLengthU32(out[i:i+4], len(tool))
	i += 4
	copy(out[i:i+len(tool)], tool)
	i += len(tool)
	putLengthU32(out[i:i+4], len(dispatch))
	i += 4
	copy(out[i:i+len(dispatch)], dispatch)
	i += len(dispatch)
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
	if rec.Tool == "" {
		rec.Tool = "unknown"
	}
	return rec
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
	r.DroppedBytes = r.RawBytes - r.KeptBytes
	if r.RawBytes > 0 {
		r.DropRatio = float64(r.DroppedBytes) / float64(r.RawBytes)
	}
	r.EstimatedInputTokens = tokensFromBytes(r.RawBytes)
	r.EstimatedOutputTokens = tokensFromBytes(r.KeptBytes)
	r.EstimatedSavedTokens = r.EstimatedInputTokens - r.EstimatedOutputTokens
	if r.EstimatedInputTokens > 0 {
		r.EstimatedSavingsPct = (float64(r.EstimatedSavedTokens) / float64(r.EstimatedInputTokens)) * 100
	}
}

func fillSummaryToolDerived(r *SummaryToolRow) {
	r.DroppedBytes = r.RawBytes - r.KeptBytes
	if r.RawBytes > 0 {
		r.DropRatio = float64(r.DroppedBytes) / float64(r.RawBytes)
	}
	r.EstimatedInputTokens = tokensFromBytes(r.RawBytes)
	r.EstimatedOutputTokens = tokensFromBytes(r.KeptBytes)
	r.EstimatedSavedTokens = r.EstimatedInputTokens - r.EstimatedOutputTokens
	if r.EstimatedInputTokens > 0 {
		r.EstimatedSavingsPct = (float64(r.EstimatedSavedTokens) / float64(r.EstimatedInputTokens)) * 100
	}
}

func fillTotalDerived(total *SummaryTotal) {
	total.DroppedBytes = total.RawBytes - total.KeptBytes
	if total.RawBytes > 0 {
		total.DropRatio = float64(total.DroppedBytes) / float64(total.RawBytes)
	}
	total.EstimatedInputTokens = tokensFromBytes(total.RawBytes)
	total.EstimatedOutputTokens = tokensFromBytes(total.KeptBytes)
	total.EstimatedSavedTokens = total.EstimatedInputTokens - total.EstimatedOutputTokens
	if total.EstimatedInputTokens > 0 {
		total.EstimatedSavingsPct = (float64(total.EstimatedSavedTokens) / float64(total.EstimatedInputTokens)) * 100
	}
}

func tokensFromBytes(v int64) int64 {
	if v <= 0 {
		return 0
	}
	return (v + 3) / 4
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

func ensureSchema(path string) (err error) {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := ensureLocalCCPGitignore(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	db, err := openDB(path, false)
	if err != nil {
		return err
	}
	defer closeBoltDBWithErr(db, &err)
	return db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(runsBucket)
		return err
	})
}

func closeBoltDBWithErr(db *bolt.DB, retErr *error) {
	if closeErr := db.Close(); *retErr == nil && closeErr != nil {
		*retErr = closeErr
	}
}

func ensureLocalCCPGitignore(path string) error {
	cleanPath := filepath.Clean(path)
	if filepath.Base(cleanPath) != "gain.db" {
		return nil
	}
	ccpDir := filepath.Dir(cleanPath)
	if filepath.Base(ccpDir) != ".ccp" {
		return nil
	}
	projectRoot := filepath.Dir(ccpDir)
	gitMeta := filepath.Join(projectRoot, ".git")
	if _, err := os.Stat(gitMeta); err != nil {
		return nil
	}

	return projectfiles.EnsureGitignoreEntry(projectRoot, ".ccp")
}

func Bootstrap(path string) error {
	return ensureSchema(path)
}

func IsTimeoutOrBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "busy") || strings.Contains(msg, "database is locked")
}
