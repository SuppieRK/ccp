package lifecycle

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-command-compression-proxy/internal/metrics"
)

type reportFlags struct {
	format string
	period string
	since  string
	tool   string
	failed bool
	table  bool
}

type filtersEnvelope struct {
	Since  string `json:"since"`
	Tool   string `json:"tool"`
	Failed bool   `json:"failed"`
}

type summaryEnvelope struct {
	Dataset string               `json:"dataset"`
	Period  string               `json:"period"`
	Filters filtersEnvelope      `json:"filters"`
	Rows    []metrics.SummaryRow `json:"rows"`
	Total   metrics.SummaryTotal `json:"total"`
}

type historyEnvelope struct {
	Dataset string               `json:"dataset"`
	Period  string               `json:"period"`
	Filters filtersEnvelope      `json:"filters"`
	Rows    []metrics.HistoryRow `json:"rows"`
}

type periodEnvelope struct {
	Dataset string              `json:"dataset"`
	Period  string              `json:"period"`
	Filters filtersEnvelope     `json:"filters"`
	Rows    []metrics.PeriodRow `json:"rows"`
}

const (
	noResultsMsg     = "No results for selected filters."
	savingsPctFormat = "~%.2f%%"
	gainHeaderText   = "ccp gain (estimated tokens: 4B/token)"
)

// RunGain prints summary/period savings in text/json/csv form.
func RunGain(args []string, metricsPath string) error {
	flags, err := parseReportFlags("gain", args)
	if err != nil {
		return err
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

// RunHistory prints execution history in text/json/csv form.
func RunHistory(args []string, metricsPath string) error {
	flags, err := parseReportFlags("history", args)
	if err != nil {
		return err
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
			Rows:    rows,
		})
	case "csv":
		return writeHistoryCSV(rows, filters)
	default:
		return printHistoryText(rows, filters)
	}
}

func parseReportFlags(name string, args []string) (reportFlags, error) {
	fs := newLifecycleFlagSet(name)
	format := fs.String("format", "text", "output format: text|json|csv")
	period := fs.String("period", "", "period aggregation: day|week|month (gain only)")
	since := fs.String("since", "", "time filter (e.g. 24h, 7d, 2w)")
	tool := fs.String("tool", "", "filter by tool")
	failed := fs.Bool("failed", false, "include only failed runs")
	table := fs.Bool("table", false, "render detailed text table output (gain only)")
	legacyJSON := fs.Bool("json", false, "emit JSON (deprecated alias for --format json)")
	setReportUsage(fs, name)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return reportFlags{}, err
	}
	if handled {
		return reportFlags{}, nil
	}
	out := reportFlags{
		format: strings.ToLower(strings.TrimSpace(*format)),
		period: strings.ToLower(strings.TrimSpace(*period)),
		since:  strings.TrimSpace(*since),
		tool:   strings.TrimSpace(*tool),
		failed: *failed,
		table:  *table,
	}
	if *legacyJSON {
		out.format = "json"
	}
	if out.format == "" {
		out.format = "text"
	}
	if err := validateReportFormat(out.format); err != nil {
		return reportFlags{}, err
	}
	if err := validateReportPeriod(name, out.period); err != nil {
		return reportFlags{}, err
	}
	if name == "history" && out.table {
		return reportFlags{}, fmt.Errorf("--table is only valid for gain")
	}
	return out, nil
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
			Rows:    rows,
		})
	case "csv":
		return writePeriodCSV(rows, opts.Period, filters)
	default:
		return printPeriodText(rows, opts.Period, filters)
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
		return printSummaryText(toolRows, total, filters)
	}
	if opts.Period != "" {
		return printWindowSummaryText(metricsPath, summaryOpts, filters, toolRows, total, opts.Period)
	}
	return printShareableSummaryText(filters, toolRows, total)
}

func setReportUsage(fs *flag.FlagSet, name string) {
	summary := "show token savings history"
	usage := []string{"ccp gain [--format text|json|csv] [--table] [--period day|week|month] [--since <duration>] [--tool <tool>] [--failed]"}
	notes := []string{
		"Use --period only with ccp gain.",
		"Run ccp gain after install or init to verify savings on real work.",
		"Default text output is a short shareable summary; use --table for detailed text tables.",
		"Legacy --json remains available as an alias for --format json.",
	}
	if name == "history" {
		summary = "show recorded command history"
		usage = []string{"ccp history [--format text|json|csv] [--since <duration>] [--tool <tool>] [--failed]"}
		notes = []string{
			"ccp history does not support --period.",
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

func printShareableSummaryText(filters filtersEnvelope, rows []metrics.SummaryToolRow, total metrics.SummaryTotal) error {
	fmt.Println(gainHeaderText)
	fmt.Printf("filters: since=%s tool=%s failed=%t period=none\n\n", displayFilter(filters.Since, "all"), displayFilter(filters.Tool, "*"), filters.Failed)
	if total.Commands == 0 {
		fmt.Println(noResultsMsg)
		return nil
	}

	fmt.Printf("- %d commands proxied, %s estimated input tokens -> %s output tokens, %s saved\n",
		total.Commands,
		formatInt(total.EstimatedInputTokens),
		formatInt(total.EstimatedOutputTokens),
		formatPercent(total.EstimatedSavingsPct),
	)
	fmt.Printf("- Biggest gains: %s\n", strongestGainsText(rows))
	fmt.Printf("- Savings held down by: %s\n", detractorsText(rows))
	fmt.Printf("- Bottom line: %s estimated tokens saved while preserving native execution semantics\n", formatInt(total.EstimatedSavedTokens))
	return nil
}

func printSummaryText(rows []metrics.SummaryToolRow, total metrics.SummaryTotal, filters filtersEnvelope) error {
	fmt.Println(gainHeaderText)
	fmt.Printf("filters: since=%s tool=%s failed=%t period=none\n\n", displayFilter(filters.Since, "all"), displayFilter(filters.Tool, "*"), filters.Failed)
	if len(rows) == 0 {
		fmt.Println(noResultsMsg)
		return nil
	}
	sortedRows := append([]metrics.SummaryToolRow(nil), rows...)
	sort.Slice(sortedRows, func(i, j int) bool {
		if sortedRows[i].Commands != sortedRows[j].Commands {
			return sortedRows[i].Commands > sortedRows[j].Commands
		}
		if sortedRows[i].EstimatedInputTokens != sortedRows[j].EstimatedInputTokens {
			return sortedRows[i].EstimatedInputTokens > sortedRows[j].EstimatedInputTokens
		}
		return sortedRows[i].Tool < sortedRows[j].Tool
	})

	toolW, countW, nativeW, proxiedW, savingsW := summaryTextColumnWidths(sortedRows, total)
	totalSavings := fmt.Sprintf(savingsPctFormat, total.EstimatedSavingsPct)

	fmt.Printf("%-*s  %*s  %*s  %*s  %*s\n", toolW, "TOOL", countW, "COUNT", nativeW, "NATIVE", proxiedW, "PROXIED", savingsW, "SAVINGS")
	printSummaryRows(sortedRows, toolW, countW, nativeW, proxiedW, savingsW)
	fmt.Printf("%-*s  %*d  %*d  %*d  %*s\n", toolW, "TOTAL", countW, total.Commands, nativeW, total.EstimatedInputTokens, proxiedW, total.EstimatedOutputTokens, savingsW, totalSavings)

	return nil
}

func summaryTextColumnWidths(rows []metrics.SummaryToolRow, total metrics.SummaryTotal) (toolW, countW, nativeW, proxiedW, savingsW int) {
	toolW = len("TOOL")
	countW = len("COUNT")
	nativeW = len("NATIVE")
	proxiedW = len("PROXIED")
	savingsW = len("SAVINGS")
	for _, r := range rows {
		toolW = max(toolW, len(r.Tool))
		countW = max(countW, len(strconv.FormatInt(r.Commands, 10)))
		nativeW = max(nativeW, len(strconv.FormatInt(r.EstimatedInputTokens, 10)))
		proxiedW = max(proxiedW, len(strconv.FormatInt(r.EstimatedOutputTokens, 10)))
		savingsW = max(savingsW, len(fmt.Sprintf(savingsPctFormat, r.EstimatedSavingsPct)))
	}
	if toolW > 20 {
		toolW = 20
	}
	if toolW < len("TOTAL") {
		toolW = len("TOTAL")
	}
	savingsW = max(savingsW, len(fmt.Sprintf(savingsPctFormat, total.EstimatedSavingsPct)))
	countW = max(countW, len(strconv.FormatInt(total.Commands, 10)))
	nativeW = max(nativeW, len(strconv.FormatInt(total.EstimatedInputTokens, 10)))
	proxiedW = max(proxiedW, len(strconv.FormatInt(total.EstimatedOutputTokens, 10)))
	return toolW, countW, nativeW, proxiedW, savingsW
}

func printSummaryRows(rows []metrics.SummaryToolRow, toolW, countW, nativeW, proxiedW, savingsW int) {
	for _, r := range rows {
		fmt.Printf("%-*s  %*d  %*d  %*d  %*s\n",
			toolW, truncateForDisplay(r.Tool, toolW),
			countW, r.Commands,
			nativeW, r.EstimatedInputTokens,
			proxiedW, r.EstimatedOutputTokens,
			savingsW, fmt.Sprintf(savingsPctFormat, r.EstimatedSavingsPct),
		)
	}
}

func printHistoryText(rows []metrics.HistoryRow, filters filtersEnvelope) error {
	fmt.Println("ccp history (estimated tokens: 4B/token)")
	fmt.Printf("filters: since=%s tool=%s failed=%t\n", displayFilter(filters.Since, "all"), displayFilter(filters.Tool, "*"), filters.Failed)
	fmt.Printf("rows: %d\n\n", len(rows))
	if len(rows) == 0 {
		fmt.Println(noResultsMsg)
		return nil
	}
	cmdW := len("COMMAND")
	for _, r := range rows {
		if l := len(r.Command); l > cmdW {
			cmdW = l
		}
	}
	if cmdW > 36 {
		cmdW = 36
	}
	fmt.Printf("%-20s  %-*s  %-11s  %s\n", "TIMESTAMP", cmdW, "COMMAND", "STATUS", "SAVINGS")
	for _, r := range rows {
		fmt.Printf("%-20s  %-*s  %-11s  %s\n",
			r.Timestamp.Format(time.RFC3339),
			cmdW, truncateForDisplay(r.Command, cmdW),
			historyStatus(r),
			fmt.Sprintf(savingsPctFormat, r.EstimatedSavingsPct),
		)
	}
	return nil
}

func printWindowSummaryText(metricsPath string, opts metrics.QueryOptions, filters filtersEnvelope, toolRows []metrics.SummaryToolRow, total metrics.SummaryTotal, period string) error {
	dayRows, err := metrics.QueryPeriod(metricsPath, windowDayQueryOptions(opts, period))
	if err != nil {
		return err
	}
	fmt.Println(gainHeaderText)
	fmt.Printf("filters: since=%s tool=%s failed=%t period=%s\n\n", displayFilter(filters.Since, "all"), displayFilter(filters.Tool, "*"), filters.Failed, period)
	if total.Commands == 0 {
		fmt.Println(noResultsMsg)
		return nil
	}

	windowLabel := map[string]string{
		"day":   "Last 24h",
		"week":  "Last 7d",
		"month": "Last 30d",
	}[period]
	if windowLabel == "" {
		windowLabel = "Selected window"
	}
	fmt.Printf("- %s: %d commands, %s estimated input tokens -> %s output tokens, %s saved\n",
		windowLabel,
		total.Commands,
		formatInt(total.EstimatedInputTokens),
		formatInt(total.EstimatedOutputTokens),
		formatPercent(total.EstimatedSavingsPct),
	)
	fmt.Printf("- Biggest gains: %s\n", strongestGainsText(toolRows))
	if busiest := busiestDayText(dayRows); busiest != "" {
		fmt.Printf("- Busiest day: %s\n", busiest)
	}
	if best := bestDayText(dayRows); best != "" {
		fmt.Printf("- Best day: %s\n", best)
	}
	if trend := recentTrendText(dayRows, period); trend != "" {
		fmt.Printf("- Recent trend: %s\n", trend)
	}
	fmt.Printf("- Savings held down by: %s\n", detractorsText(toolRows))
	return nil
}

func printPeriodText(rows []metrics.PeriodRow, period string, filters filtersEnvelope) error {
	fmt.Println(gainHeaderText)
	fmt.Printf("filters: since=%s tool=%s failed=%t period=%s\n\n", displayFilter(filters.Since, "all"), displayFilter(filters.Tool, "*"), filters.Failed, period)
	if len(rows) == 0 {
		fmt.Println(noResultsMsg)
		return nil
	}
	fmt.Printf("%-10s  %-10s  %-10s  %-8s  %s\n", "BUCKET", "START", "END", "COUNT", "SAVINGS")
	for _, r := range rows {
		fmt.Printf("%-10s  %-10s  %-10s  %-8d  %s\n",
			r.Bucket,
			r.BucketStart,
			r.BucketEnd,
			r.Commands,
			fmt.Sprintf(savingsPctFormat, r.EstimatedSavingsPct),
		)
	}
	return nil
}

func windowDayQueryOptions(opts metrics.QueryOptions, period string) metrics.QueryOptions {
	windowed := opts
	windowed.Period = "day"
	windowed.Since = effectiveWindowSince(opts.Since, period)
	return windowed
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

func strongestGainsText(rows []metrics.SummaryToolRow) string {
	sorted := append([]metrics.SummaryToolRow(nil), rows...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].EstimatedSavedTokens != sorted[j].EstimatedSavedTokens {
			return sorted[i].EstimatedSavedTokens > sorted[j].EstimatedSavedTokens
		}
		if sorted[i].EstimatedSavingsPct != sorted[j].EstimatedSavingsPct {
			return sorted[i].EstimatedSavingsPct > sorted[j].EstimatedSavingsPct
		}
		return sorted[i].Tool < sorted[j].Tool
	})
	parts := make([]string, 0, 3)
	for _, row := range sorted {
		if row.EstimatedSavedTokens <= 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s (%d cmds)", row.Tool, formatPercent(row.EstimatedSavingsPct), row.Commands))
		if len(parts) == 3 {
			break
		}
	}
	if len(parts) == 0 {
		return "no material gains in the selected dataset"
	}
	return strings.Join(parts, ", ")
}

func detractorsText(rows []metrics.SummaryToolRow) string {
	sorted := append([]metrics.SummaryToolRow(nil), rows...)
	sort.Slice(sorted, func(i, j int) bool {
		iLow := sorted[i].EstimatedSavingsPct <= 5
		jLow := sorted[j].EstimatedSavingsPct <= 5
		if iLow != jLow {
			return iLow
		}
		if sorted[i].Commands != sorted[j].Commands {
			return sorted[i].Commands > sorted[j].Commands
		}
		if sorted[i].EstimatedSavingsPct != sorted[j].EstimatedSavingsPct {
			return sorted[i].EstimatedSavingsPct < sorted[j].EstimatedSavingsPct
		}
		return sorted[i].Tool < sorted[j].Tool
	})
	parts := make([]string, 0, 3)
	for _, row := range sorted {
		if row.Commands == 0 {
			continue
		}
		if row.EstimatedSavingsPct > 25 && len(parts) > 0 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s %s (%d cmds)", row.Tool, formatPercent(row.EstimatedSavingsPct), row.Commands))
		if len(parts) == 3 {
			break
		}
	}
	if len(parts) == 0 {
		return "no clear detractors in the selected dataset"
	}
	return strings.Join(parts, ", ")
}

func busiestDayText(rows []metrics.PeriodRow) string {
	if len(rows) == 0 {
		return ""
	}
	best := rows[0]
	for _, row := range rows[1:] {
		if row.Commands > best.Commands || (row.Commands == best.Commands && row.BucketStart > best.BucketStart) {
			best = row
		}
	}
	return fmt.Sprintf("%s with %d commands", best.BucketStart, best.Commands)
}

func bestDayText(rows []metrics.PeriodRow) string {
	if len(rows) == 0 {
		return ""
	}
	best := rows[0]
	for _, row := range rows[1:] {
		if row.EstimatedSavingsPct > best.EstimatedSavingsPct || (row.EstimatedSavingsPct == best.EstimatedSavingsPct && row.BucketStart > best.BucketStart) {
			best = row
		}
	}
	return fmt.Sprintf("%s at %s", best.BucketStart, formatPercent(best.EstimatedSavingsPct))
}

func recentTrendText(rows []metrics.PeriodRow, period string) string {
	if len(rows) < 2 {
		return "not enough daily history for a trend comparison"
	}
	sorted := append([]metrics.PeriodRow(nil), rows...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].BucketStart < sorted[j].BucketStart })
	split := trendSplitIndex(len(sorted), period)
	if split <= 0 || split >= len(sorted) {
		return "not enough daily history for a trend comparison"
	}
	earlier := averageSavings(sorted[:split])
	recent := averageSavings(sorted[split:])
	diff := recent - earlier
	switch {
	case diff > 0.01:
		return fmt.Sprintf("%s over %d recent buckets, up %.2f points from the earlier window", formatPercent(recent), len(sorted)-split, diff)
	case diff < -0.01:
		return fmt.Sprintf("%s over %d recent buckets, down %.2f points from the earlier window", formatPercent(recent), len(sorted)-split, -diff)
	default:
		return fmt.Sprintf("%s over %d recent buckets, flat against the earlier window", formatPercent(recent), len(sorted)-split)
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

func formatPercent(v float64) string {
	return fmt.Sprintf(savingsPctFormat, v)
}

func formatInt(v int64) string {
	return strconv.FormatInt(v, 10)
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
