package lifecycle

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-command-compression-proxy/internal/metrics"
	"go-command-compression-proxy/internal/workspaces"
)

type globalHistoryRow struct {
	metrics.HistoryRow
	Source string `json:"source"`
}

type globalHistoryEnvelope struct {
	Dataset string                `json:"dataset"`
	Period  string                `json:"period"`
	Filters filtersEnvelope       `json:"filters"`
	Storage metrics.StorageStatus `json:"storage"`
	Rows    []globalHistoryRow    `json:"rows"`
}

type globalMetricsSource struct {
	CWD         string
	MetricsPath string
}

type globalQueryFailure struct {
	CWD         string
	MetricsPath string
	Err         error
}

type globalQuerySession struct {
	sources  []globalMetricsSource
	failures map[string]globalQueryFailure
}

func runGlobalGain(flags reportFlags, opts metrics.QueryOptions, filters filtersEnvelope, currentMetricsPath string) error {
	session, err := newGlobalQuerySession(currentMetricsPath)
	if err != nil {
		return err
	}
	defer session.writeWarnings("gain")

	if shouldRenderPeriodDataset(flags, opts) {
		return renderGlobalPeriodGain(session, flags, opts, filters)
	}
	return renderGlobalSummaryGain(session, flags, opts, filters)
}

func renderGlobalPeriodGain(session *globalQuerySession, flags reportFlags, opts metrics.QueryOptions, filters filtersEnvelope) error {
	rows, err := queryGlobalPeriodRows(session, opts)
	if err != nil {
		return err
	}
	switch flags.format {
	case "json":
		return writeJSON(periodEnvelope{
			Dataset: "period",
			Period:  opts.Period,
			Filters: filters,
			Storage: session.storageStatus(),
			Rows:    rows,
		})
	case "csv":
		return writePeriodCSV(rows, opts.Period, filters)
	default:
		return printPeriodText(rows, opts.Period, filters, flags.limit)
	}
}

func renderGlobalSummaryGain(session *globalQuerySession, flags reportFlags, opts metrics.QueryOptions, filters filtersEnvelope) error {
	summaryOpts := summaryQueryOptions(flags, opts)
	rows, err := queryGlobalSummaryRows(session, summaryOpts)
	if err != nil {
		return err
	}
	total := totalFromSummaryRows(rows)

	switch flags.format {
	case "json":
		return writeJSON(summaryEnvelope{
			Dataset: "summary",
			Period:  "",
			Filters: filters,
			Storage: session.storageStatus(),
			Rows:    rows,
			Total:   total,
		})
	case "csv":
		return writeSummaryCSV(rows, total, filters)
	default:
		return renderGlobalSummaryText(session, flags, opts, filters, total, summaryOpts)
	}
}

func renderGlobalSummaryText(session *globalQuerySession, flags reportFlags, opts metrics.QueryOptions, filters filtersEnvelope, total metrics.SummaryTotal, summaryOpts metrics.QueryOptions) error {
	toolRows, err := queryGlobalSummaryToolRows(session, summaryOpts)
	if err != nil {
		return err
	}
	if flags.table {
		return printSummaryTableText(filters, total, toolRows, flags.limit, opts.Period, true)
	}
	trendPeriod := defaultGainTrendPeriod(opts.Period)
	dayRows, err := queryGlobalTrendRows(session, opts, trendPeriod)
	if err != nil {
		return err
	}
	return printCompactGainSummary(filters, total, toolRows, dayRows, trendPeriod, opts.Period, true)
}

func queryGlobalTrendRows(session *globalQuerySession, opts metrics.QueryOptions, period string) ([]metrics.PeriodRow, error) {
	return queryTrendPeriodRows(func(queryOpts metrics.QueryOptions) ([]metrics.PeriodRow, error) {
		return queryGlobalPeriodRows(session, queryOpts)
	}, opts, period, gainNow())
}

func runGlobalHistory(flags reportFlags, opts metrics.QueryOptions, filters filtersEnvelope, currentMetricsPath string) error {
	session, err := newGlobalQuerySession(currentMetricsPath)
	if err != nil {
		return err
	}
	defer session.writeWarnings("history")

	opts.Period = ""
	rows, err := queryGlobalHistoryRows(session, opts)
	if err != nil {
		return err
	}
	switch flags.format {
	case "json":
		return writeJSON(globalHistoryEnvelope{
			Dataset: "history",
			Period:  "",
			Filters: filters,
			Storage: session.storageStatus(),
			Rows:    rows,
		})
	case "csv":
		return writeGlobalHistoryCSV(rows, filters)
	default:
		return printGlobalHistoryTable(rows, filters, flags.limit)
	}
}

func newGlobalQuerySession(currentMetricsPath string) (*globalQuerySession, error) {
	sources, err := globalMetricsSources(currentMetricsPath)
	if err != nil {
		return nil, err
	}
	return &globalQuerySession{
		sources:  sources,
		failures: map[string]globalQueryFailure{},
	}, nil
}

func (s *globalQuerySession) recordFailure(source globalMetricsSource, err error) {
	if s == nil || err == nil {
		return
	}
	key := source.MetricsPath
	if key == "" {
		key = source.CWD
	}
	if _, exists := s.failures[key]; exists {
		return
	}
	s.failures[key] = globalQueryFailure{
		CWD:         source.CWD,
		MetricsPath: source.MetricsPath,
		Err:         err,
	}
}

func (s *globalQuerySession) storageStatus() metrics.StorageStatus {
	var total metrics.StorageStatus
	if s == nil {
		return total
	}
	for _, source := range s.sources {
		current := metrics.InspectStorage(source.MetricsPath)
		total.Observed += current.Observed
		total.Pending += current.Pending
		total.Rejected += current.Rejected
		total.StorageErrors += current.StorageErrors
	}
	return total
}

func (s *globalQuerySession) writeWarnings(command string) {
	if s == nil || len(s.failures) == 0 {
		return
	}
	failures := make([]globalQueryFailure, 0, len(s.failures))
	for _, failure := range s.failures {
		failures = append(failures, failure)
	}
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].CWD != failures[j].CWD {
			return failures[i].CWD < failures[j].CWD
		}
		return failures[i].MetricsPath < failures[j].MetricsPath
	})
	for _, failure := range failures {
		writeLifecycleWarning("ccp %s --global: warning: skipped workspace %s (%s): %v\n", command, globalQuerySourceLabel(failure), failure.MetricsPath, failure.Err)
	}
	writeLifecycleWarning("ccp %s --global: warning: results exclude %d workspace(s) with unreadable or corrupt metrics\n", command, len(failures))
}

func globalQuerySourceLabel(failure globalQueryFailure) string {
	if failure.CWD != "" {
		return failure.CWD
	}
	if failure.MetricsPath != "" {
		return failure.MetricsPath
	}
	return "<unknown>"
}

func queryGlobalSummaryRows(session *globalQuerySession, opts metrics.QueryOptions) ([]metrics.SummaryRow, error) {
	grouped := map[string]metrics.SummaryRow{}
	for _, source := range session.sources {
		rows, err := metrics.QuerySummaryRows(source.MetricsPath, opts)
		if err != nil {
			session.recordFailure(source, err)
			continue
		}
		for _, row := range rows {
			current := grouped[row.Command]
			current.Command = row.Command
			current.Commands += row.Commands
			current.RawBytes += row.RawBytes
			current.KeptBytes += row.KeptBytes
			grouped[row.Command] = current
		}
	}
	out := make([]metrics.SummaryRow, 0, len(grouped))
	for _, row := range grouped {
		fillLocalSummaryRowDerived(&row)
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Commands != out[j].Commands {
			return out[i].Commands > out[j].Commands
		}
		if out[i].EstimatedInputTokens != out[j].EstimatedInputTokens {
			return out[i].EstimatedInputTokens > out[j].EstimatedInputTokens
		}
		return out[i].Command < out[j].Command
	})
	return out, nil
}

func queryGlobalSummaryToolRows(session *globalQuerySession, opts metrics.QueryOptions) ([]metrics.SummaryToolRow, error) {
	grouped := map[string]metrics.SummaryToolRow{}
	for _, source := range session.sources {
		rows, err := metrics.QuerySummaryRowsByTool(source.MetricsPath, opts)
		if err != nil {
			session.recordFailure(source, err)
			continue
		}
		for _, row := range rows {
			current := grouped[row.Tool]
			current.Tool = row.Tool
			current.Commands += row.Commands
			current.RawBytes += row.RawBytes
			current.KeptBytes += row.KeptBytes
			grouped[row.Tool] = current
		}
	}
	out := make([]metrics.SummaryToolRow, 0, len(grouped))
	for _, row := range grouped {
		fillLocalSummaryToolDerived(&row)
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Commands != out[j].Commands {
			return out[i].Commands > out[j].Commands
		}
		if out[i].EstimatedInputTokens != out[j].EstimatedInputTokens {
			return out[i].EstimatedInputTokens > out[j].EstimatedInputTokens
		}
		return out[i].Tool < out[j].Tool
	})
	return out, nil
}

func queryGlobalPeriodRows(session *globalQuerySession, opts metrics.QueryOptions) ([]metrics.PeriodRow, error) {
	grouped := map[string]metrics.PeriodRow{}
	for _, source := range session.sources {
		rows, err := metrics.QueryPeriod(source.MetricsPath, opts)
		if err != nil {
			session.recordFailure(source, err)
			continue
		}
		for _, row := range rows {
			current := grouped[row.Bucket]
			current.Bucket = row.Bucket
			current.BucketStart = row.BucketStart
			current.BucketEnd = row.BucketEnd
			current.Commands += row.Commands
			current.RawBytes += row.RawBytes
			current.KeptBytes += row.KeptBytes
			grouped[row.Bucket] = current
		}
	}
	out := make([]metrics.PeriodRow, 0, len(grouped))
	for _, row := range grouped {
		fillLocalPeriodRowDerived(&row)
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].BucketStart < out[j].BucketStart
	})
	return out, nil
}

func queryGlobalHistoryRows(session *globalQuerySession, opts metrics.QueryOptions) ([]globalHistoryRow, error) {
	rows := make([]globalHistoryRow, 0, 32)
	for _, source := range session.sources {
		historyRows, err := metrics.QueryHistory(source.MetricsPath, opts)
		if err != nil {
			session.recordFailure(source, err)
			continue
		}
		for _, row := range historyRows {
			rows = append(rows, globalHistoryRow{
				HistoryRow: row,
				Source:     source.CWD,
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Timestamp.Equal(rows[j].Timestamp) {
			if rows[i].Source != rows[j].Source {
				return rows[i].Source < rows[j].Source
			}
			return rows[i].Command < rows[j].Command
		}
		return rows[i].Timestamp.After(rows[j].Timestamp)
	})
	return rows, nil
}

func globalMetricsSources(currentMetricsPath string) ([]globalMetricsSource, error) {
	registryPath, err := workspaces.DefaultPath()
	entries := []workspaces.Workspace(nil)
	if err == nil {
		entries, err = workspaces.ListPath(registryPath)
	}
	if err != nil {
		entries = nil
	}

	sources := make([]globalMetricsSource, 0, len(entries)+1)
	seenMetricsPaths := make(map[string]struct{}, len(entries)+1)
	add := func(cwd, metricsPath string) {
		normalizedMetricsPath := normalizeGlobalPath(metricsPath)
		if normalizedMetricsPath == "" {
			return
		}
		if _, err := os.Stat(normalizedMetricsPath); err != nil {
			return
		}
		if _, ok := seenMetricsPaths[normalizedMetricsPath]; ok {
			return
		}
		seenMetricsPaths[normalizedMetricsPath] = struct{}{}
		sources = append(sources, globalMetricsSource{
			CWD:         normalizeGlobalPath(cwd),
			MetricsPath: normalizedMetricsPath,
		})
	}

	for _, entry := range entries {
		add(entry.CWD, entry.MetricsPath)
	}

	if current := currentGlobalMetricsSource(currentMetricsPath); current != nil {
		add(current.CWD, current.MetricsPath)
		if current.CWD != "" && current.MetricsPath != "" && registryPath != "" {
			_ = workspaces.UpsertPath(registryPath, current.CWD, current.MetricsPath)
		}
	}

	return sources, nil
}

func currentGlobalMetricsSource(currentMetricsPath string) *globalMetricsSource {
	normalizedMetricsPath := normalizeGlobalPath(currentMetricsPath)
	if normalizedMetricsPath == "" {
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return &globalMetricsSource{MetricsPath: normalizedMetricsPath}
	}
	return &globalMetricsSource{
		CWD:         normalizeGlobalPath(cwd),
		MetricsPath: normalizedMetricsPath,
	}
}

func normalizeGlobalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func totalFromSummaryRows(rows []metrics.SummaryRow) metrics.SummaryTotal {
	var total metrics.SummaryTotal
	for _, row := range rows {
		total.Commands += row.Commands
		total.RawBytes += row.RawBytes
		total.KeptBytes += row.KeptBytes
	}
	fillLocalSummaryTotalDerived(&total)
	return total
}

func fillLocalSummaryRowDerived(row *metrics.SummaryRow) {
	metrics.FillSummaryDerived(row)
}

func fillLocalSummaryToolDerived(row *metrics.SummaryToolRow) {
	metrics.FillSummaryToolDerived(row)
}

func fillLocalSummaryTotalDerived(total *metrics.SummaryTotal) {
	metrics.FillTotalDerived(total)
}

func fillLocalPeriodRowDerived(row *metrics.PeriodRow) {
	metrics.FillPeriodDerived(row)
}

func localTokensFromBytes(v int64) int64 {
	return metrics.TokensFromBytes(v)
}

func writeGlobalHistoryCSV(rows []globalHistoryRow, filters filtersEnvelope) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()
	header := []string{
		"dataset", "period", "since", "tool_filter", "failed_filter", "row_kind",
		"timestamp", "source", "command", "tool", "dispatch_key", "exit_code", "failed", "passthrough", "duration_ms",
		"commands", "raw_bytes", "kept_bytes", "dropped_bytes", "drop_ratio",
		"estimated_input_tokens", "estimated_output_tokens", "estimated_saved_tokens", "estimated_savings_pct",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, row := range rows {
		rec := []string{
			"history", "", filters.Since, filters.Tool, strconv.FormatBool(filters.Failed), "data",
			row.Timestamp.Format(time.RFC3339), row.Source, row.Command, row.Tool, row.DispatchKey,
			strconv.Itoa(row.ExitCode), strconv.FormatBool(row.Failed), strconv.FormatBool(row.Passthrough), strconv.FormatInt(row.DurationMS, 10),
			"",
			strconv.FormatInt(row.RawBytes, 10),
			strconv.FormatInt(row.KeptBytes, 10),
			strconv.FormatInt(row.DroppedBytes, 10),
			fmt.Sprintf("%.4f", row.DropRatio),
			strconv.FormatInt(row.EstimatedInputTokens, 10),
			strconv.FormatInt(row.EstimatedOutputTokens, 10),
			strconv.FormatInt(row.EstimatedSavedTokens, 10),
			fmt.Sprintf("%.2f", row.EstimatedSavingsPct),
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	return nil
}

func truncateTailForDisplay(input string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(input) <= max {
		return input
	}
	if max <= 3 {
		return input[len(input)-max:]
	}
	return "..." + input[len(input)-max+3:]
}
