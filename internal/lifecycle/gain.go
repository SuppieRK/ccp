package lifecycle

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/SuppieRK/cmdshape/internal/metrics"
)

type reportFlags struct {
	format string
	period string
	since  string
	tool   string
	failed bool
	table  bool
	global bool
	limit  int
}

type filtersEnvelope struct {
	Since  string `json:"since"`
	Tool   string `json:"tool"`
	Failed bool   `json:"failed"`
}

type summaryEnvelope struct {
	Dataset string                `json:"dataset"`
	Period  string                `json:"period"`
	Filters filtersEnvelope       `json:"filters"`
	Storage metrics.StorageStatus `json:"storage"`
	Rows    []metrics.SummaryRow  `json:"rows"`
	Total   metrics.SummaryTotal  `json:"total"`
}

type historyEnvelope struct {
	Dataset string                `json:"dataset"`
	Period  string                `json:"period"`
	Filters filtersEnvelope       `json:"filters"`
	Storage metrics.StorageStatus `json:"storage"`
	Rows    []metrics.HistoryRow  `json:"rows"`
}

type periodEnvelope struct {
	Dataset string                `json:"dataset"`
	Period  string                `json:"period"`
	Filters filtersEnvelope       `json:"filters"`
	Storage metrics.StorageStatus `json:"storage"`
	Rows    []metrics.PeriodRow   `json:"rows"`
}

const (
	noResultsMsg     = "No results for selected filters."
	savingsPctFormat = "~%.2f%%"
	gainHeaderText   = "cmdshape gain (estimated tokens: 4B/token)"
)

var gainNow = func() time.Time {
	return time.Now().UTC()
}

func RunGain(args []string, metricsPath string) error {
	flags, handled, err := parseReportFlags("gain", args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	if err := validateGainFlags(flags); err != nil {
		return err
	}
	opts, err := buildQueryOptions(flags)
	if err != nil {
		return err
	}
	filters := filtersEnvelope{
		Since:  flags.since,
		Tool:   flags.tool,
		Failed: flags.failed,
	}
	if flags.global {
		return runGlobalGain(flags, opts, filters, metricsPath)
	}
	if shouldRenderPeriodDataset(flags, opts) {
		return renderPeriodDataset(metricsPath, flags, opts, filters)
	}
	summaryOpts := summaryQueryOptions(flags, opts)
	total, err := metrics.QuerySummary(metricsPath, summaryOpts)
	if err != nil {
		return err
	}
	return renderSummaryDataset(metricsPath, flags, opts, summaryOpts, filters, total)
}

func RunHistory(args []string, metricsPath string) error {
	if len(args) > 0 && args[0] == "purge" {
		return runHistoryPurge(args[1:], metricsPath)
	}
	flags, handled, err := parseReportFlags("history", args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	opts, err := buildQueryOptions(flags)
	if err != nil {
		return err
	}
	opts.Period = ""
	filters := filtersEnvelope{
		Since:  flags.since,
		Tool:   flags.tool,
		Failed: flags.failed,
	}
	if flags.global {
		return runGlobalHistory(flags, opts, filters, metricsPath)
	}
	rows, err := metrics.QueryHistory(metricsPath, opts)
	if err != nil {
		return err
	}
	switch flags.format {
	case "json":
		return writeJSON(historyEnvelope{
			Dataset: "history",
			Period:  "",
			Filters: filters,
			Storage: metrics.InspectStorage(metricsPath),
			Rows:    rows,
		})
	case "csv":
		return writeHistoryCSV(rows, filters)
	default:
		return printHistoryTable(rows, filters, flags.limit)
	}
}

func runHistoryPurge(args []string, metricsPath string) error {
	request, handled, err := parseHistoryPurgeRequest(args)
	if err != nil || handled {
		return err
	}
	sources, err := historyPurgeSources(metricsPath, request.global)
	if err != nil {
		return err
	}
	removed, err := purgeHistorySources(sources, request.cutoff)
	if err != nil {
		return err
	}
	fmt.Printf("Purged %d history records before %s.\n", removed, request.cutoff.UTC().Format(time.RFC3339))
	return nil
}

type historyPurgeRequest struct {
	cutoff time.Time
	global bool
}

func parseHistoryPurgeRequest(args []string) (historyPurgeRequest, bool, error) {
	fs := newLifecycleFlagSet("history purge")
	before := fs.String("before", "", "remove history older than this duration (for example 90d)")
	global := fs.Bool("global", false, "purge every registered workspace")
	yes := fs.Bool("yes", false, "confirm destructive history removal")
	setLifecycleUsage(
		fs,
		"remove recorded command history older than a duration",
		[]string{"cmdshape history purge --before <duration> [--global] --yes"},
		"Records at the exact cutoff are retained.",
		"--global applies the same cutoff to registered workspace databases.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return historyPurgeRequest{}, false, err
	}
	if handled {
		return historyPurgeRequest{}, true, nil
	}
	if len(fs.Args()) != 0 {
		return historyPurgeRequest{}, false, fmt.Errorf("history purge does not accept positional arguments")
	}
	if strings.TrimSpace(*before) == "" {
		return historyPurgeRequest{}, false, fmt.Errorf("history purge requires --before <duration>")
	}
	duration, err := parseSince(*before)
	if err != nil || duration <= 0 {
		return historyPurgeRequest{}, false, fmt.Errorf("invalid --before %q", *before)
	}
	if !*yes {
		return historyPurgeRequest{}, false, fmt.Errorf("history purge requires --yes")
	}
	return historyPurgeRequest{
		cutoff: gainNow().Add(-duration),
		global: *global,
	}, false, nil
}

func historyPurgeSources(metricsPath string, global bool) ([]globalMetricsSource, error) {
	sources := []globalMetricsSource{currentGlobalMetricsSourceValue(metricsPath)}
	if !global {
		return sources, nil
	}
	return globalMetricsSources(metricsPath)
}

func purgeHistorySources(sources []globalMetricsSource, cutoff time.Time) (int, error) {
	removed := 0
	for _, source := range sources {
		if strings.TrimSpace(source.MetricsPath) == "" {
			continue
		}
		count, err := purgeHistorySource(source, cutoff)
		if err != nil {
			return removed, fmt.Errorf("purge history %q: %w", source.MetricsPath, err)
		}
		removed += count
	}
	return removed, nil
}

func purgeHistorySource(source globalMetricsSource, cutoff time.Time) (int, error) {
	if projectRoot := containedMetricsProject(source.CWD, source.MetricsPath); projectRoot != "" {
		return metrics.PurgeProjectBefore(projectRoot, source.MetricsPath, cutoff)
	}
	return metrics.PurgeBefore(source.MetricsPath, cutoff)
}

func currentGlobalMetricsSourceValue(metricsPath string) globalMetricsSource {
	if source := currentGlobalMetricsSource(metricsPath); source != nil {
		return *source
	}
	return globalMetricsSource{}
}

func containedMetricsProject(cwd, metricsPath string) string {
	if strings.TrimSpace(cwd) == "" || strings.TrimSpace(metricsPath) == "" {
		return ""
	}
	root, err := filepath.Abs(filepath.Clean(cwd))
	if err != nil {
		return ""
	}
	path, err := filepath.Abs(filepath.Clean(metricsPath))
	if err != nil || path != metrics.ProjectPath(root) {
		return ""
	}
	return root
}

func parseReportFlags(name string, args []string) (reportFlags, bool, error) {
	fs := newLifecycleFlagSet(name)
	format := fs.String("format", "text", "output format: text|json|csv")
	period := fs.String("period", "", "period aggregation: day|week|month (gain only)")
	since := fs.String("since", "", "time filter (e.g. 24h, 7d, 2w)")
	tool := fs.String("tool", "", "filter by tool")
	failed := fs.Bool("failed", false, "include only failed runs")
	table := fs.Bool("table", false, "render detailed text table output (gain only)")
	global := fs.Bool("global", false, "aggregate across registered workspace metrics databases")
	limit := fs.Int("limit", -1, "limit rows in detailed text views (text output only, 0 = unlimited)")
	legacyJSON := fs.Bool("json", false, "emit JSON (deprecated alias for --format json)")
	setReportUsage(fs, name)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return reportFlags{}, false, err
	}
	if handled {
		return reportFlags{}, true, nil
	}
	out := reportFlags{
		format: strings.ToLower(strings.TrimSpace(*format)),
		period: strings.ToLower(strings.TrimSpace(*period)),
		since:  strings.TrimSpace(*since),
		tool:   strings.TrimSpace(*tool),
		failed: *failed,
		table:  *table,
		global: *global,
		limit:  *limit,
	}
	if *legacyJSON {
		out.format = "json"
	}
	if out.format == "" {
		out.format = "text"
	}
	if err := validateReportFormat(out.format); err != nil {
		return reportFlags{}, false, err
	}
	if err := validateReportPeriod(name, out.period); err != nil {
		return reportFlags{}, false, err
	}
	if name == "history" && out.table {
		return reportFlags{}, false, fmt.Errorf("--table is only valid for gain")
	}
	if out.limit < -1 {
		return reportFlags{}, false, fmt.Errorf("invalid --limit %d (expected -1, 0, or a positive integer)", out.limit)
	}
	return out, false, nil
}

func validateGainFlags(flags reportFlags) error {
	if flags.table && flags.format != "text" {
		return fmt.Errorf("--table is only valid with text output")
	}
	return nil
}

func shouldRenderPeriodDataset(flags reportFlags, opts metrics.QueryOptions) bool {
	return opts.Period != "" && (flags.format == "json" || flags.format == "csv" || flags.table)
}

func summaryQueryOptions(flags reportFlags, opts metrics.QueryOptions) metrics.QueryOptions {
	summaryOpts := opts
	if opts.Period != "" && flags.format == "text" && !flags.table {
		summaryOpts.Period = ""
		summaryOpts.Since = effectiveWindowSince(opts.Since, opts.Period)
	}
	return summaryOpts
}

func renderPeriodDataset(metricsPath string, flags reportFlags, opts metrics.QueryOptions, filters filtersEnvelope) error {
	rows, err := metrics.QueryPeriod(metricsPath, opts)
	if err != nil {
		return err
	}
	switch flags.format {
	case "json":
		return writeJSON(periodEnvelope{
			Dataset: "period",
			Period:  opts.Period,
			Filters: filters,
			Storage: metrics.InspectStorage(metricsPath),
			Rows:    rows,
		})
	case "csv":
		return writePeriodCSV(rows, opts.Period, filters)
	default:
		return printPeriodText(rows, opts.Period, filters, flags.limit)
	}
}

func renderSummaryDataset(metricsPath string, flags reportFlags, opts, summaryOpts metrics.QueryOptions, filters filtersEnvelope, total metrics.SummaryTotal) error {
	switch flags.format {
	case "json":
		rows, err := metrics.QuerySummaryRows(metricsPath, summaryOpts)
		if err != nil {
			return err
		}
		return writeJSON(summaryEnvelope{
			Dataset: "summary",
			Period:  "",
			Filters: filters,
			Storage: metrics.InspectStorage(metricsPath),
			Rows:    rows,
			Total:   total,
		})
	case "csv":
		rows, err := metrics.QuerySummaryRows(metricsPath, summaryOpts)
		if err != nil {
			return err
		}
		return writeSummaryCSV(rows, total, filters)
	default:
		return renderTextSummaryDataset(metricsPath, flags, opts, summaryOpts, filters, total)
	}
}

func renderTextSummaryDataset(metricsPath string, flags reportFlags, opts, summaryOpts metrics.QueryOptions, filters filtersEnvelope, total metrics.SummaryTotal) error {
	toolRows, err := metrics.QuerySummaryRowsByTool(metricsPath, summaryOpts)
	if err != nil {
		return err
	}
	if flags.table {
		return printSummaryTableText(filters, total, toolRows, flags.limit, opts.Period, false)
	}
	trendPeriod := defaultGainTrendPeriod(opts.Period)
	trendRows, err := queryTrendRows(metricsPath, opts, trendPeriod)
	if err != nil {
		return err
	}
	return printCompactGainSummary(filters, total, toolRows, trendRows, trendPeriod, opts.Period, false)
}

func queryTrendRows(metricsPath string, opts metrics.QueryOptions, period string) ([]metrics.PeriodRow, error) {
	return queryTrendPeriodRows(func(queryOpts metrics.QueryOptions) ([]metrics.PeriodRow, error) {
		return metrics.QueryPeriod(metricsPath, queryOpts)
	}, opts, period, gainNow())
}

type periodRowQuery func(metrics.QueryOptions) ([]metrics.PeriodRow, error)

func queryTrendPeriodRows(query periodRowQuery, opts metrics.QueryOptions, period string, now time.Time) ([]metrics.PeriodRow, error) {
	dayRows, err := query(trendDayQueryOptions(opts, period, now))
	if err != nil {
		return nil, err
	}
	return aggregateTrendRows(dayRows, period, now), nil
}

func trendDayQueryOptions(opts metrics.QueryOptions, period string, now time.Time) metrics.QueryOptions {
	windowed := opts
	windowed.Period = "day"
	earliestStart, _, _, ok := trendWindowBounds(now, period)
	if !ok {
		return windowed
	}
	lookback := now.UTC().Sub(earliestStart)
	if opts.Since > 0 && opts.Since < lookback {
		windowed.Since = opts.Since
		return windowed
	}
	windowed.Since = lookback
	return windowed
}

func aggregateTrendRows(dayRows []metrics.PeriodRow, period string, now time.Time) []metrics.PeriodRow {
	earliestStart, recentStart, recentEnd, ok := trendWindowBounds(now, period)
	if !ok {
		return nil
	}
	earliestEnd := recentStart.AddDate(0, 0, -1)
	recentEndExclusive := recentEnd.AddDate(0, 0, 1)
	earliest := metrics.PeriodRow{
		Bucket:      earliestStart.Format(time.DateOnly),
		BucketStart: earliestStart.Format(time.DateOnly),
		BucketEnd:   earliestEnd.Format(time.DateOnly),
	}
	recent := metrics.PeriodRow{
		Bucket:      recentStart.Format(time.DateOnly),
		BucketStart: recentStart.Format(time.DateOnly),
		BucketEnd:   recentEnd.Format(time.DateOnly),
	}
	for _, row := range dayRows {
		bucketStart, err := time.Parse(time.DateOnly, row.BucketStart)
		if err != nil {
			continue
		}
		switch {
		case !bucketStart.Before(earliestStart) && bucketStart.Before(recentStart):
			addTrendPeriodRow(&earliest, row)
		case !bucketStart.Before(recentStart) && bucketStart.Before(recentEndExclusive):
			addTrendPeriodRow(&recent, row)
		}
	}
	out := make([]metrics.PeriodRow, 0, 2)
	if earliest.Commands > 0 {
		fillLocalPeriodRowDerived(&earliest)
		out = append(out, earliest)
	}
	if recent.Commands > 0 {
		fillLocalPeriodRowDerived(&recent)
		out = append(out, recent)
	}
	return out
}

func addTrendPeriodRow(dst *metrics.PeriodRow, src metrics.PeriodRow) {
	dst.Commands += src.Commands
	dst.RawBytes += src.RawBytes
	dst.KeptBytes += src.KeptBytes
}

func trendWindowBounds(now time.Time, period string) (time.Time, time.Time, time.Time, bool) {
	windowDays := trendWindowDays(period)
	if windowDays <= 0 {
		return time.Time{}, time.Time{}, time.Time{}, false
	}
	recentEnd := dayStartUTC(now)
	recentStart := recentEnd.AddDate(0, 0, -(windowDays - 1))
	earliestEnd := recentStart.AddDate(0, 0, -1)
	earliestStart := earliestEnd.AddDate(0, 0, -(windowDays - 1))
	return earliestStart, recentStart, recentEnd, true
}

func trendWindowDays(period string) int {
	switch period {
	case "day":
		return 1
	case "week":
		return 7
	case "month":
		return 30
	default:
		return 0
	}
}

func dayStartUTC(value time.Time) time.Time {
	utc := value.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func setReportUsage(fs *flag.FlagSet, name string) {
	summary := "show token savings history"
	usage := []string{"cmdshape gain [--format text|json|csv] [--table] [--limit <n>] [--period day|week|month] [--since <duration>] [--tool <tool>] [--failed] [--global]"}
	notes := []string{
		"Use --period only with cmdshape gain.",
		"Run cmdshape gain after install or init to verify savings on real work.",
		"Use --global to aggregate across registered workspace metrics databases.",
		"Default text output is a short shareable summary; use --table for detailed text tables.",
		"--limit applies to text output only; detailed text views default to 15 rows.",
		"Use --limit 0 for unlimited text rows; negative values are invalid except -1 (default behavior).",
		"Legacy --json remains available as an alias for --format json.",
	}
	if name == "history" {
		summary = "show recorded command history"
		usage = []string{"cmdshape history [--format text|json|csv] [--limit <n>] [--since <duration>] [--tool <tool>] [--failed] [--global]"}
		notes = []string{
			"cmdshape history does not support --period.",
			"Use --global to merge history from registered workspace metrics databases; global rows include a source field.",
			"--limit applies to text output only; cmdshape history defaults to 15 rows.",
			"Use --limit 0 for unlimited text rows; negative values are invalid except -1 (default behavior).",
			"Legacy --json remains available as an alias for --format json.",
		}
	}
	setLifecycleUsage(fs, summary, usage, notes...)
}

func validateReportFormat(format string) error {
	switch format {
	case "text", "json", "csv":
		return nil
	default:
		return fmt.Errorf("invalid --format %q (expected text|json|csv)", format)
	}
}

func validateReportPeriod(commandName, period string) error {
	if commandName == "history" && period != "" {
		return fmt.Errorf("--period is only valid for gain")
	}
	if period == "" {
		return nil
	}
	switch period {
	case "day", "week", "month":
		return nil
	default:
		return fmt.Errorf("invalid --period %q (expected day|week|month)", period)
	}
}

func buildQueryOptions(flags reportFlags) (metrics.QueryOptions, error) {
	opts := metrics.QueryOptions{
		Tool:   flags.tool,
		Failed: flags.failed,
		Period: flags.period,
	}
	if flags.since == "" {
		return opts, nil
	}
	dur, err := parseSince(flags.since)
	if err != nil {
		return metrics.QueryOptions{}, err
	}
	opts.Since = dur
	return opts, nil
}

func parseSince(raw string) (time.Duration, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return 0, nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	unit := s[len(s)-1]
	num := s[:len(s)-1]
	v, err := strconv.ParseInt(num, 10, 64)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("invalid --since %q", raw)
	}
	switch unit {
	case 'd':
		return time.Duration(v) * 24 * time.Hour, nil
	case 'w':
		return time.Duration(v) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid --since %q", raw)
	}
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printPeriodText(rows []metrics.PeriodRow, period string, filters filtersEnvelope, limit int) error {
	fmt.Println(gainHeaderText)
	fmt.Printf("filters: since=%s tool=%s failed=%t period=%s\n\n", displayFilter(filters.Since, "all"), displayFilter(filters.Tool, "*"), filters.Failed, period)
	if len(rows) == 0 {
		fmt.Println(noResultsMsg)
		return nil
	}

	limitedRows, totalRows := limitRows(rows, limit)
	fmt.Println(tableSummaryLine(len(limitedRows), totalRows, "buckets"))
	fmt.Println()
	fmt.Printf("%-10s  %-10s  %-10s  %-8s  %s\n", "BUCKET", "START", "END", "COUNT", "SAVINGS")
	for _, r := range limitedRows {
		fmt.Printf("%-10s  %-10s  %-10s  %-8s  %s\n",
			r.Bucket,
			r.BucketStart,
			r.BucketEnd,
			formatInt(r.Commands),
			fmt.Sprintf(savingsPctFormat, r.EstimatedSavingsPct),
		)
	}
	return nil
}

func effectiveWindowSince(base time.Duration, period string) time.Duration {
	window := durationForPeriod(period)
	if window <= 0 {
		return base
	}
	if base <= 0 || base > window {
		return window
	}
	return base
}

func durationForPeriod(period string) time.Duration {
	switch period {
	case "day":
		return 24 * time.Hour
	case "week":
		return 7 * 24 * time.Hour
	case "month":
		return 30 * 24 * time.Hour
	default:
		return 0
	}
}

func trendSplitIndex(n int, period string) int {
	switch period {
	case "week":
		if n < 3 {
			return 0
		}
		if n <= 4 {
			return n / 2
		}
		return n - 3
	case "month":
		if n < 4 {
			return 0
		}
		if n > 7 {
			return n - 7
		}
		return n / 2
	case "day":
		return n / 2
	default:
		return n / 2
	}
}

func averageSavings(rows []metrics.PeriodRow) float64 {
	if len(rows) == 0 {
		return 0
	}
	var sum float64
	for _, row := range rows {
		sum += row.EstimatedSavingsPct
	}
	return sum / float64(len(rows))
}

func formatInt(v int64) string {
	s := strconv.FormatInt(v, 10)
	unsigned, neg := strings.CutPrefix(s, "-")
	if len(s) <= 3 && !neg {
		return s
	}
	if neg {
		s = unsigned
	}
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	head := len(s) % 3
	if head == 0 {
		head = 3
	}
	b.WriteString(s[:head])
	for i := head; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func writeSummaryCSV(rows []metrics.SummaryRow, total metrics.SummaryTotal, filters filtersEnvelope) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()
	header := []string{
		"dataset", "period", "since", "tool_filter", "failed_filter", "row_kind",
		"timestamp", "command", "tool", "dispatch_key", "exit_code", "failed", "passthrough", "duration_ms",
		"commands", "raw_bytes", "kept_bytes", "dropped_bytes", "drop_ratio",
		"estimated_input_tokens", "estimated_output_tokens", "estimated_saved_tokens", "estimated_savings_pct",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		rec := []string{
			"summary", "", filters.Since, filters.Tool, strconv.FormatBool(filters.Failed), "data",
			"", r.Command, "", "", "", "", "", "",
			strconv.FormatInt(r.Commands, 10),
			strconv.FormatInt(r.RawBytes, 10),
			strconv.FormatInt(r.KeptBytes, 10),
			strconv.FormatInt(r.DroppedBytes, 10),
			fmt.Sprintf("%.4f", r.DropRatio),
			strconv.FormatInt(r.EstimatedInputTokens, 10),
			strconv.FormatInt(r.EstimatedOutputTokens, 10),
			strconv.FormatInt(r.EstimatedSavedTokens, 10),
			fmt.Sprintf("%.2f", r.EstimatedSavingsPct),
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	totalRec := []string{
		"summary", "", filters.Since, filters.Tool, strconv.FormatBool(filters.Failed), "total",
		"", "", "", "", "", "", "", "",
		strconv.FormatInt(total.Commands, 10),
		strconv.FormatInt(total.RawBytes, 10),
		strconv.FormatInt(total.KeptBytes, 10),
		strconv.FormatInt(total.DroppedBytes, 10),
		fmt.Sprintf("%.4f", total.DropRatio),
		strconv.FormatInt(total.EstimatedInputTokens, 10),
		strconv.FormatInt(total.EstimatedOutputTokens, 10),
		strconv.FormatInt(total.EstimatedSavedTokens, 10),
		fmt.Sprintf("%.2f", total.EstimatedSavingsPct),
	}
	return w.Write(totalRec)
}

func writeHistoryCSV(rows []metrics.HistoryRow, filters filtersEnvelope) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()
	header := []string{
		"dataset", "period", "since", "tool_filter", "failed_filter", "row_kind",
		"timestamp", "command", "tool", "dispatch_key", "exit_code", "failed", "passthrough", "duration_ms",
		"commands", "raw_bytes", "kept_bytes", "dropped_bytes", "drop_ratio",
		"estimated_input_tokens", "estimated_output_tokens", "estimated_saved_tokens", "estimated_savings_pct",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		rec := []string{
			"history", "", filters.Since, filters.Tool, strconv.FormatBool(filters.Failed), "data",
			r.Timestamp.Format(time.RFC3339), r.Command, r.Tool, r.DispatchKey,
			strconv.Itoa(r.ExitCode), strconv.FormatBool(r.Failed), strconv.FormatBool(r.Passthrough), strconv.FormatInt(r.DurationMS, 10),
			"",
			strconv.FormatInt(r.RawBytes, 10),
			strconv.FormatInt(r.KeptBytes, 10),
			strconv.FormatInt(r.DroppedBytes, 10),
			fmt.Sprintf("%.4f", r.DropRatio),
			strconv.FormatInt(r.EstimatedInputTokens, 10),
			strconv.FormatInt(r.EstimatedOutputTokens, 10),
			strconv.FormatInt(r.EstimatedSavedTokens, 10),
			fmt.Sprintf("%.2f", r.EstimatedSavingsPct),
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	return nil
}

func writePeriodCSV(rows []metrics.PeriodRow, period string, filters filtersEnvelope) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()
	header := []string{
		"dataset", "period", "since", "tool_filter", "failed_filter", "bucket", "bucket_start", "bucket_end",
		"commands", "raw_bytes", "kept_bytes", "dropped_bytes", "drop_ratio",
		"estimated_input_tokens", "estimated_output_tokens", "estimated_saved_tokens", "estimated_savings_pct",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		rec := []string{
			"period", period, filters.Since, filters.Tool, strconv.FormatBool(filters.Failed),
			r.Bucket, r.BucketStart, r.BucketEnd,
			strconv.FormatInt(r.Commands, 10),
			strconv.FormatInt(r.RawBytes, 10),
			strconv.FormatInt(r.KeptBytes, 10),
			strconv.FormatInt(r.DroppedBytes, 10),
			fmt.Sprintf("%.4f", r.DropRatio),
			strconv.FormatInt(r.EstimatedInputTokens, 10),
			strconv.FormatInt(r.EstimatedOutputTokens, 10),
			strconv.FormatInt(r.EstimatedSavedTokens, 10),
			fmt.Sprintf("%.2f", r.EstimatedSavingsPct),
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	return nil
}

func displayFilter(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func historyStatus(r metrics.HistoryRow) string {
	if r.Passthrough {
		return "passthrough"
	}
	if r.Failed {
		return "failed"
	}
	return "ok"
}

func truncateForDisplay(s string, max int) string {
	if max <= 0 {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	if max <= 3 {
		return string(rs[:max])
	}
	return string(rs[:max-3]) + "..."
}
