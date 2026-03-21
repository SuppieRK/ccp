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
	Dataset string             `json:"dataset"`
	Period  string             `json:"period"`
	Filters filtersEnvelope    `json:"filters"`
	Rows    []globalHistoryRow `json:"rows"`
}

type globalMetricsSource struct {
	CWD         string
	MetricsPath string
}

func runGlobalGain(flags reportFlags, opts metrics.QueryOptions, filters filtersEnvelope, currentMetricsPath string) error {
	if shouldRenderPeriodDataset(flags, opts) {
		return renderGlobalPeriodGain(flags, opts, filters, currentMetricsPath)
	}
	return renderGlobalSummaryGain(flags, opts, filters, currentMetricsPath)
}

func renderGlobalPeriodGain(flags reportFlags, opts metrics.QueryOptions, filters filtersEnvelope, currentMetricsPath string) error {
	rows, err := queryGlobalPeriodRows(opts, currentMetricsPath)
	if err != nil {
		return err
	}
	switch flags.format {
	case "json":
		return writeJSON(periodEnvelope{
			Dataset: "period",
			Period:  opts.Period,
			Filters: filters,
			Rows:    rows,
		})
	case "csv":
		return writePeriodCSV(rows, opts.Period, filters)
	default:
		return printPeriodText(rows, opts.Period, filters)
	}
}

func renderGlobalSummaryGain(flags reportFlags, opts metrics.QueryOptions, filters filtersEnvelope, currentMetricsPath string) error {
	summaryOpts := summaryQueryOptions(flags, opts)
	rows, err := queryGlobalSummaryRows(summaryOpts, currentMetricsPath)
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
			Rows:    rows,
			Total:   total,
		})
	case "csv":
		return writeSummaryCSV(rows, total, filters)
	default:
		return renderGlobalSummaryText(flags, opts, filters, total, summaryOpts, currentMetricsPath)
	}
}

func renderGlobalSummaryText(flags reportFlags, opts metrics.QueryOptions, filters filtersEnvelope, total metrics.SummaryTotal, summaryOpts metrics.QueryOptions, currentMetricsPath string) error {
	toolRows, err := queryGlobalSummaryToolRows(summaryOpts, currentMetricsPath)
	if err != nil {
		return err
	}
	if flags.table {
		return printSummaryTableText(filters, total, toolRows, flags.limit, opts.Period, true)
	}
	trendPeriod := defaultGainTrendPeriod(opts.Period)
	dayRows, err := queryGlobalPeriodRows(windowDayQueryOptions(summaryOpts, trendPeriod), currentMetricsPath)
	if err != nil {
		return err
	}
	return printCompactGainSummary(filters, total, toolRows, dayRows, trendPeriod, opts.Period, true)
}

func runGlobalHistory(flags reportFlags, opts metrics.QueryOptions, filters filtersEnvelope, currentMetricsPath string) error {
	opts.Period = ""
	rows, err := queryGlobalHistoryRows(opts, currentMetricsPath)
	if err != nil {
		return err
	}
	switch flags.format {
	case "json":
		return writeJSON(globalHistoryEnvelope{
			Dataset: "history",
			Period:  "",
			Filters: filters,
			Rows:    rows,
		})
	case "csv":
		return writeGlobalHistoryCSV(rows, filters)
	default:
		return printGlobalHistoryTable(rows, filters, flags.limit)
	}
}

func queryGlobalSummaryRows(opts metrics.QueryOptions, currentMetricsPath string) ([]metrics.SummaryRow, error) {
	sources, err := globalMetricsSources(currentMetricsPath)
	if err != nil {
		return nil, err
	}
	grouped := map[string]metrics.SummaryRow{}
	for _, source := range sources {
		rows, err := metrics.QuerySummaryRows(source.MetricsPath, opts)
		if err != nil {
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

func queryGlobalSummaryToolRows(opts metrics.QueryOptions, currentMetricsPath string) ([]metrics.SummaryToolRow, error) {
	sources, err := globalMetricsSources(currentMetricsPath)
	if err != nil {
		return nil, err
	}
	grouped := map[string]metrics.SummaryToolRow{}
	for _, source := range sources {
		rows, err := metrics.QuerySummaryRowsByTool(source.MetricsPath, opts)
		if err != nil {
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

func queryGlobalPeriodRows(opts metrics.QueryOptions, currentMetricsPath string) ([]metrics.PeriodRow, error) {
	sources, err := globalMetricsSources(currentMetricsPath)
	if err != nil {
		return nil, err
	}
	grouped := map[string]metrics.PeriodRow{}
	for _, source := range sources {
		rows, err := metrics.QueryPeriod(source.MetricsPath, opts)
		if err != nil {
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

func queryGlobalHistoryRows(opts metrics.QueryOptions, currentMetricsPath string) ([]globalHistoryRow, error) {
	sources, err := globalMetricsSources(currentMetricsPath)
	if err != nil {
		return nil, err
	}
	rows := make([]globalHistoryRow, 0, 32)
	for _, source := range sources {
		historyRows, err := metrics.QueryHistory(source.MetricsPath, opts)
		if err != nil {
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
	entries, err := workspaces.List()
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
		if current.CWD != "" && current.MetricsPath != "" {
			_ = workspaces.Upsert(current.CWD, current.MetricsPath)
		}
	}

	return sources, nil
}

func currentGlobalMetricsSource(currentMetricsPath string) *globalMetricsSource {
	normalizedMetricsPath := normalizeGlobalPath(currentMetricsPath)
	if normalizedMetricsPath == "" {
		return nil
	}
	cwd, err := initDetectRoot()
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
	row.DroppedBytes = row.RawBytes - row.KeptBytes
	if row.RawBytes > 0 {
		row.DropRatio = float64(row.DroppedBytes) / float64(row.RawBytes)
	}
	row.EstimatedInputTokens = localTokensFromBytes(row.RawBytes)
	row.EstimatedOutputTokens = localTokensFromBytes(row.KeptBytes)
	row.EstimatedSavedTokens = row.EstimatedInputTokens - row.EstimatedOutputTokens
	if row.EstimatedInputTokens > 0 {
		row.EstimatedSavingsPct = (float64(row.EstimatedSavedTokens) / float64(row.EstimatedInputTokens)) * 100
	}
}

func fillLocalSummaryToolDerived(row *metrics.SummaryToolRow) {
	row.DroppedBytes = row.RawBytes - row.KeptBytes
	if row.RawBytes > 0 {
		row.DropRatio = float64(row.DroppedBytes) / float64(row.RawBytes)
	}
	row.EstimatedInputTokens = localTokensFromBytes(row.RawBytes)
	row.EstimatedOutputTokens = localTokensFromBytes(row.KeptBytes)
	row.EstimatedSavedTokens = row.EstimatedInputTokens - row.EstimatedOutputTokens
	if row.EstimatedInputTokens > 0 {
		row.EstimatedSavingsPct = (float64(row.EstimatedSavedTokens) / float64(row.EstimatedInputTokens)) * 100
	}
}

func fillLocalSummaryTotalDerived(total *metrics.SummaryTotal) {
	total.DroppedBytes = total.RawBytes - total.KeptBytes
	if total.RawBytes > 0 {
		total.DropRatio = float64(total.DroppedBytes) / float64(total.RawBytes)
	}
	total.EstimatedInputTokens = localTokensFromBytes(total.RawBytes)
	total.EstimatedOutputTokens = localTokensFromBytes(total.KeptBytes)
	total.EstimatedSavedTokens = total.EstimatedInputTokens - total.EstimatedOutputTokens
	if total.EstimatedInputTokens > 0 {
		total.EstimatedSavingsPct = (float64(total.EstimatedSavedTokens) / float64(total.EstimatedInputTokens)) * 100
	}
}

func fillLocalPeriodRowDerived(row *metrics.PeriodRow) {
	row.DroppedBytes = row.RawBytes - row.KeptBytes
	if row.RawBytes > 0 {
		row.DropRatio = float64(row.DroppedBytes) / float64(row.RawBytes)
	}
	row.EstimatedInputTokens = localTokensFromBytes(row.RawBytes)
	row.EstimatedOutputTokens = localTokensFromBytes(row.KeptBytes)
	row.EstimatedSavedTokens = row.EstimatedInputTokens - row.EstimatedOutputTokens
	if row.EstimatedInputTokens > 0 {
		row.EstimatedSavingsPct = (float64(row.EstimatedSavedTokens) / float64(row.EstimatedInputTokens)) * 100
	}
}

func localTokensFromBytes(v int64) int64 {
	if v <= 0 {
		return 0
	}
	return (v + 3) / 4
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
