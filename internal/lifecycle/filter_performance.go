package lifecycle

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"go-command-compression-proxy/internal/metrics"
)

const (
	filterPerformanceDataset = "filter-performance"
	maxPerformanceHints      = 5
)

type filterPerformanceFlags struct {
	format string
	since  string
	tool   string
	failed bool
	global bool
	limit  int
}

type filterPerformanceEnvelope struct {
	Dataset     string                           `json:"dataset"`
	Filters     filtersEnvelope                  `json:"filters"`
	Rows        []metrics.PerformanceRow         `json:"rows"`
	Build       metrics.RegistryBuildSummary     `json:"build"`
	BuildRows   []metrics.RegistrySourceBuildRow `json:"build_rows"`
	Suggestions []filterPerformanceSuggestion    `json:"suggestions"`
}

type filterPerformanceSuggestion struct {
	Kind       string  `json:"kind"`
	Tool       string  `json:"tool"`
	Filter     string  `json:"filter"`
	Case       string  `json:"case"`
	Command    string  `json:"command"`
	Count      int64   `json:"count"`
	Reason     string  `json:"reason"`
	SavingsPct float64 `json:"savings_pct"`
}

type performanceGlobalAcc struct {
	row        metrics.PerformanceRow
	durationMS float64
}

type filterPerformanceData struct {
	rows         []metrics.PerformanceRow
	missed       []metrics.MissedOpportunity
	buildSummary metrics.RegistryBuildSummary
	buildRows    []metrics.RegistrySourceBuildRow
}

func RunFilterPerformance(args []string, metricsPath string) error {
	flags, handled, err := parseFilterPerformanceFlags(args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	opts, err := buildFilterPerformanceQueryOptions(flags)
	if err != nil {
		return err
	}
	filters := filtersEnvelope{
		Since:  flags.since,
		Tool:   flags.tool,
		Failed: flags.failed,
	}
	data, err := queryFilterPerformance(metricsPath, flags, opts)
	if err != nil {
		return err
	}
	suggestions := buildFilterPerformanceSuggestions(data.rows, data.missed)
	switch flags.format {
	case "json":
		return writeJSON(filterPerformanceEnvelope{
			Dataset:     filterPerformanceDataset,
			Filters:     filters,
			Rows:        data.rows,
			Build:       data.buildSummary,
			BuildRows:   data.buildRows,
			Suggestions: suggestions,
		})
	case "csv":
		return writeFilterPerformanceCSV(data.rows, data.buildSummary, data.buildRows, suggestions, filters)
	default:
		return printFilterPerformanceText(data.rows, data.buildSummary, data.buildRows, suggestions, filters, flags.limit, flags.global)
	}
}

func parseFilterPerformanceFlags(args []string) (filterPerformanceFlags, bool, error) {
	fs := newLifecycleFlagSet("filter performance")
	format := fs.String("format", "text", "output format: text|json|csv")
	since := fs.String("since", "", "time filter (e.g. 24h, 7d, 2w)")
	tool := fs.String("tool", "", "filter by invoked tool")
	failed := fs.Bool("failed", false, "include only failed runs")
	global := fs.Bool("global", false, "aggregate across registered workspace metrics databases")
	limit := fs.Int("limit", -1, "limit rows in text output only, 0 = unlimited")
	legacyJSON := fs.Bool("json", false, "emit JSON (deprecated alias for --format json)")
	setLifecycleUsage(
		fs,
		"show YAML filter and case performance",
		[]string{"ccp filter performance [--format text|json|csv] [--limit <n>] [--since <duration>] [--tool <tool>] [--failed] [--global]"},
		"Rows are grouped by invoked tool, resolved filter, case, and recorded filter provenance.",
		"Existing metrics recorded before provenance support show blank source/path/hash fields.",
		"Use --tool <tool> for focused improvements; tool means the invoked command name.",
		"Use --global to aggregate across registered workspace metrics databases.",
		"Pair this report with 'ccp filter prompt <name>' before editing filters.",
		"--limit applies to text output only; text output defaults to 15 rows.",
		"Legacy --json remains available as an alias for --format json.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return filterPerformanceFlags{}, false, err
	}
	if handled {
		return filterPerformanceFlags{}, true, nil
	}
	out := filterPerformanceFlags{
		format: strings.ToLower(strings.TrimSpace(*format)),
		since:  strings.TrimSpace(*since),
		tool:   strings.TrimSpace(*tool),
		failed: *failed,
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
		return filterPerformanceFlags{}, false, err
	}
	if out.limit < -1 {
		return filterPerformanceFlags{}, false, fmt.Errorf("invalid --limit %d (expected -1, 0, or a positive integer)", out.limit)
	}
	if fs.NArg() != 0 {
		return filterPerformanceFlags{}, false, fmt.Errorf("filter performance does not accept positional arguments")
	}
	return out, false, nil
}

func buildFilterPerformanceQueryOptions(flags filterPerformanceFlags) (metrics.QueryOptions, error) {
	opts := metrics.QueryOptions{
		Tool:   flags.tool,
		Failed: flags.failed,
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

func queryFilterPerformance(metricsPath string, flags filterPerformanceFlags, opts metrics.QueryOptions) (filterPerformanceData, error) {
	if flags.global {
		session, err := newGlobalQuerySession(metricsPath)
		if err != nil {
			return filterPerformanceData{}, err
		}
		defer session.writeWarnings("filter performance")
		rows, err := queryGlobalPerformanceRows(session, opts)
		if err != nil {
			return filterPerformanceData{}, err
		}
		missed, err := queryGlobalMissedOpportunities(session, opts, maxPerformanceHints)
		if err != nil {
			return filterPerformanceData{}, err
		}
		buildSummary, buildRows, err := queryGlobalRegistryBuild(session, opts)
		if err != nil {
			return filterPerformanceData{}, err
		}
		return filterPerformanceData{
			rows:         rows,
			missed:       missed,
			buildSummary: buildSummary,
			buildRows:    buildRows,
		}, nil
	}
	rows, err := metrics.QueryPerformanceRows(metricsPath, opts)
	if err != nil {
		return filterPerformanceData{}, err
	}
	missed, err := metrics.QueryMissedOpportunities(metricsPath, opts, maxPerformanceHints)
	if err != nil {
		return filterPerformanceData{}, err
	}
	buildSummary, buildRows, err := metrics.QueryRegistryBuild(metricsPath, opts)
	if err != nil {
		return filterPerformanceData{}, err
	}
	return filterPerformanceData{
		rows:         rows,
		missed:       missed,
		buildSummary: buildSummary,
		buildRows:    buildRows,
	}, nil
}

func queryGlobalPerformanceRows(session *globalQuerySession, opts metrics.QueryOptions) ([]metrics.PerformanceRow, error) {
	grouped := map[string]*performanceGlobalAcc{}
	for _, source := range session.sources {
		rows, err := metrics.QueryPerformanceRows(source.MetricsPath, opts)
		if err != nil {
			session.recordFailure(source, err)
			continue
		}
		for _, row := range rows {
			key := performanceRowKey(row)
			acc := grouped[key]
			if acc == nil {
				acc = &performanceGlobalAcc{row: performanceRowIdentity(row)}
				grouped[key] = acc
			}
			acc.row.Commands += row.Commands
			acc.row.PassthroughCommands += row.PassthroughCommands
			acc.row.FailedCommands += row.FailedCommands
			acc.row.RawBytes += row.RawBytes
			acc.row.KeptBytes += row.KeptBytes
			acc.durationMS += row.AvgDurationMS * float64(row.Commands)
		}
	}
	out := make([]metrics.PerformanceRow, 0, len(grouped))
	for _, acc := range grouped {
		row := acc.row
		metrics.FillPerformanceDerived(&row, int64(math.Round(acc.durationMS)))
		out = append(out, row)
	}
	sortPerformanceRows(out)
	return out, nil
}

func queryGlobalMissedOpportunities(session *globalQuerySession, opts metrics.QueryOptions, limit int) ([]metrics.MissedOpportunity, error) {
	grouped := map[string]int64{}
	for _, source := range session.sources {
		rows, err := metrics.QueryHistory(source.MetricsPath, opts)
		if err != nil {
			session.recordFailure(source, err)
			continue
		}
		for _, row := range rows {
			if row.Passthrough {
				grouped[row.Command]++
			}
		}
	}
	out := make([]metrics.MissedOpportunity, 0, len(grouped))
	for command, count := range grouped {
		out = append(out, metrics.MissedOpportunity{Command: command, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Command < out[j].Command
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func queryGlobalRegistryBuild(session *globalQuerySession, opts metrics.QueryOptions) (metrics.RegistryBuildSummary, []metrics.RegistrySourceBuildRow, error) {
	events := make([]metrics.RegistryBuildEvent, 0, 32)
	for _, source := range session.sources {
		rows, err := metrics.QueryRegistryBuildEvents(source.MetricsPath, opts)
		if err != nil {
			session.recordFailure(source, err)
			continue
		}
		events = append(events, rows...)
	}
	return metrics.RegistryBuildSummaryFromEvents(events), metrics.RegistrySourceBuildRowsFromEvents(events), nil
}

func performanceRowKey(row metrics.PerformanceRow) string {
	return strings.Join([]string{
		row.Tool,
		row.Filter,
		row.Case,
		row.DispatchKey,
		row.FilterSourceKind,
		row.FilterPath,
		row.FilterHash,
	}, "\x00")
}

func performanceRowIdentity(row metrics.PerformanceRow) metrics.PerformanceRow {
	return metrics.PerformanceRow{
		Tool:             row.Tool,
		Filter:           row.Filter,
		Case:             row.Case,
		DispatchKey:      row.DispatchKey,
		FilterSourceKind: row.FilterSourceKind,
		FilterPath:       row.FilterPath,
		FilterHash:       row.FilterHash,
	}
}

func sortPerformanceRows(rows []metrics.PerformanceRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Commands != rows[j].Commands {
			return rows[i].Commands > rows[j].Commands
		}
		if rows[i].EstimatedSavedTokens != rows[j].EstimatedSavedTokens {
			return rows[i].EstimatedSavedTokens > rows[j].EstimatedSavedTokens
		}
		if rows[i].Tool != rows[j].Tool {
			return rows[i].Tool < rows[j].Tool
		}
		if rows[i].Filter != rows[j].Filter {
			return rows[i].Filter < rows[j].Filter
		}
		return rows[i].Case < rows[j].Case
	})
}

func buildFilterPerformanceSuggestions(rows []metrics.PerformanceRow, missed []metrics.MissedOpportunity) []filterPerformanceSuggestion {
	suggestions := make([]filterPerformanceSuggestion, 0, maxPerformanceHints*3)
	for _, row := range rows {
		if row.Commands > 0 && row.PassthroughCommands < row.Commands && row.EstimatedSavingsPct < 5 {
			suggestions = append(suggestions, filterPerformanceSuggestion{
				Kind:       "review-case",
				Tool:       row.Tool,
				Filter:     row.Filter,
				Case:       row.Case,
				Count:      row.Commands,
				Reason:     "matched case has low or no estimated savings",
				SavingsPct: row.EstimatedSavingsPct,
			})
			if countKind(suggestions, "review-case") == maxPerformanceHints {
				break
			}
		}
	}
	for _, row := range rows {
		if row.Commands > 0 && row.FailedRate >= 0.5 {
			suggestions = append(suggestions, filterPerformanceSuggestion{
				Kind:       "failure-heavy",
				Tool:       row.Tool,
				Filter:     row.Filter,
				Case:       row.Case,
				Count:      row.FailedCommands,
				Reason:     "most recorded runs for this row failed",
				SavingsPct: row.EstimatedSavingsPct,
			})
			if countKind(suggestions, "failure-heavy") == maxPerformanceHints {
				break
			}
		}
	}
	for _, row := range missed {
		suggestions = append(suggestions, filterPerformanceSuggestion{
			Kind:    "passthrough-opportunity",
			Command: row.Command,
			Count:   row.Count,
			Reason:  "frequent passthrough command",
		})
		if countKind(suggestions, "passthrough-opportunity") == maxPerformanceHints {
			break
		}
	}
	return suggestions
}

func countKind(suggestions []filterPerformanceSuggestion, kind string) int {
	count := 0
	for _, suggestion := range suggestions {
		if suggestion.Kind == kind {
			count++
		}
	}
	return count
}

func printFilterPerformanceText(rows []metrics.PerformanceRow, buildSummary metrics.RegistryBuildSummary, buildRows []metrics.RegistrySourceBuildRow, suggestions []filterPerformanceSuggestion, filters filtersEnvelope, limit int, global bool) error {
	title := "ccp filter performance"
	if global {
		title += compactFilterSuffix(filters, "global")
	} else {
		title += compactFilterSuffix(filters)
	}
	fmt.Println(title)
	fmt.Println()
	if len(rows) == 0 {
		fmt.Println(noResultsMsg)
	} else {
		limitedRows, totalRows := limitRows(rows, limit)
		fmt.Println(tableSummaryLine(len(limitedRows), totalRows, "rows"))
		fmt.Println()

		tableRows := make([][]string, 0, len(limitedRows))
		for _, row := range limitedRows {
			tableRows = append(tableRows, []string{
				truncateForDisplay(row.Tool, 12),
				truncateForDisplay(row.Filter, 12),
				truncateForDisplay(displayPerformanceCase(row.Case), 16),
				truncateTailForDisplay(displayPerformancePath(row), 28),
				formatInt(row.Commands),
				formatCompactSavedTokens(row.EstimatedSavedTokens),
				formatPercentText(row.EstimatedSavingsPct),
				formatPercentText(row.PassthroughRate * 100),
				formatPercentText(row.FailedRate * 100),
			})
		}
		fmt.Print(renderTextTable([]textTableColumn{
			{header: "TOOL"},
			{header: "FILTER"},
			{header: "CASE"},
			{header: "PATH"},
			{header: "RUNS", right: true},
			{header: "SAVED", right: true},
			{header: "SAVINGS", right: true},
			{header: "PASS", right: true},
			{header: "FAIL", right: true},
		}, tableRows))
	}
	printRegistryBuildText(buildSummary, buildRows, limit)
	printFilterPerformanceSuggestions(suggestions)
	return nil
}

func printRegistryBuildText(summary metrics.RegistryBuildSummary, rows []metrics.RegistrySourceBuildRow, limit int) {
	if summary.Builds == 0 {
		return
	}
	fmt.Println()
	fmt.Println("Filter build")
	fmt.Printf("builds=%s avg=%s p95=%s max=%s\n",
		formatInt(summary.Builds),
		formatDurationMS(summary.AvgDurationMS),
		formatDurationMS(float64(summary.P95DurationMS)),
		formatDurationMS(float64(summary.MaxDurationMS)),
	)
	if len(rows) == 0 {
		return
	}
	limitedRows, totalRows := limitRows(rows, limit)
	fmt.Println()
	fmt.Println(tableSummaryLine(len(limitedRows), totalRows, "sources"))
	fmt.Println()
	tableRows := make([][]string, 0, len(limitedRows))
	for _, row := range limitedRows {
		tableRows = append(tableRows, []string{
			truncateForDisplay(displayRegistrySourceKind(row.SourceKind), 12),
			truncateTailForDisplay(displayRegistrySourcePath(row.SourceDir), 36),
			formatInt(row.Builds),
			formatInt(row.Errors),
			formatInt(row.Definitions),
			formatInt(row.Compiled),
			formatDurationMS(row.AvgDurationMS),
			formatDurationMS(float64(row.MaxDurationMS)),
		})
	}
	fmt.Print(renderTextTable([]textTableColumn{
		{header: "SOURCE"},
		{header: "DIR"},
		{header: "BUILDS", right: true},
		{header: "ERR", right: true},
		{header: "DEFS", right: true},
		{header: "COMPILED", right: true},
		{header: "AVG", right: true},
		{header: "MAX", right: true},
	}, tableRows))
}

func formatDurationMS(value float64) string {
	return fmt.Sprintf("%.1fms", value)
}

func displayRegistrySourceKind(kind string) string {
	if strings.TrimSpace(kind) == "" {
		return "-"
	}
	return kind
}

func displayRegistrySourcePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "-"
	}
	return compactFilterStatusPath(path)
}

func printFilterPerformanceSuggestions(suggestions []filterPerformanceSuggestion) {
	if len(suggestions) == 0 {
		return
	}
	fmt.Println()
	fmt.Println("Hints")
	for _, suggestion := range suggestions {
		fmt.Printf("- %s: %s\n", suggestion.Kind, suggestionText(suggestion))
	}
}

func suggestionText(suggestion filterPerformanceSuggestion) string {
	switch suggestion.Kind {
	case "review-case", "failure-heavy":
		return fmt.Sprintf("%s%s (%s, %s runs)", suggestion.Filter, performanceCaseSuffix(suggestion.Case), suggestion.Reason, formatInt(suggestion.Count))
	case "passthrough-opportunity":
		return fmt.Sprintf("%s (%s runs, %s)", suggestion.Command, formatInt(suggestion.Count), suggestion.Reason)
	default:
		return suggestion.Reason
	}
}

func performanceCaseSuffix(caseID string) string {
	if strings.TrimSpace(caseID) == "" {
		return ""
	}
	return "|" + caseID
}

func displayPerformanceCase(caseID string) string {
	if strings.TrimSpace(caseID) == "" {
		return "-"
	}
	return caseID
}

func displayPerformancePath(row metrics.PerformanceRow) string {
	if strings.TrimSpace(row.FilterPath) == "" {
		if strings.TrimSpace(row.FilterSourceKind) == "" {
			return "-"
		}
		return row.FilterSourceKind
	}
	return compactFilterStatusPath(row.FilterPath)
}

func writeFilterPerformanceCSV(rows []metrics.PerformanceRow, buildSummary metrics.RegistryBuildSummary, buildRows []metrics.RegistrySourceBuildRow, suggestions []filterPerformanceSuggestion, filters filtersEnvelope) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()
	header := []string{
		"dataset", "since", "tool_filter", "failed_filter", "row_kind",
		"tool", "filter", "case", "dispatch_key", "filter_source_kind", "filter_path", "filter_hash",
		"commands", "passthrough_commands", "passthrough_rate", "failed_commands", "failed_rate", "avg_duration_ms",
		"raw_bytes", "kept_bytes", "dropped_bytes", "drop_ratio",
		"estimated_input_tokens", "estimated_output_tokens", "estimated_saved_tokens", "estimated_savings_pct",
		"suggestion_kind", "suggestion_command", "suggestion_reason",
		"builds", "build_avg_duration_ms", "build_p95_duration_ms", "build_max_duration_ms",
		"build_source_kind", "build_source_dir", "build_errors", "build_definitions", "build_compiled",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, row := range rows {
		if err := w.Write(filterPerformanceRowCSV(row, filters)); err != nil {
			return err
		}
	}
	if buildSummary.Builds > 0 {
		if err := w.Write(filterPerformanceBuildSummaryCSV(buildSummary, filters)); err != nil {
			return err
		}
	}
	for _, row := range buildRows {
		if err := w.Write(filterPerformanceBuildSourceCSV(row, filters)); err != nil {
			return err
		}
	}
	for _, suggestion := range suggestions {
		if err := w.Write(filterPerformanceSuggestionCSV(suggestion, filters)); err != nil {
			return err
		}
	}
	return nil
}

func filterPerformanceRowCSV(row metrics.PerformanceRow, filters filtersEnvelope) []string {
	return []string{
		filterPerformanceDataset, filters.Since, filters.Tool, strconv.FormatBool(filters.Failed), "data",
		row.Tool, row.Filter, row.Case, row.DispatchKey, row.FilterSourceKind, row.FilterPath, row.FilterHash,
		strconv.FormatInt(row.Commands, 10),
		strconv.FormatInt(row.PassthroughCommands, 10),
		fmt.Sprintf("%.4f", row.PassthroughRate),
		strconv.FormatInt(row.FailedCommands, 10),
		fmt.Sprintf("%.4f", row.FailedRate),
		fmt.Sprintf("%.2f", row.AvgDurationMS),
		strconv.FormatInt(row.RawBytes, 10),
		strconv.FormatInt(row.KeptBytes, 10),
		strconv.FormatInt(row.DroppedBytes, 10),
		fmt.Sprintf("%.4f", row.DropRatio),
		strconv.FormatInt(row.EstimatedInputTokens, 10),
		strconv.FormatInt(row.EstimatedOutputTokens, 10),
		strconv.FormatInt(row.EstimatedSavedTokens, 10),
		fmt.Sprintf("%.2f", row.EstimatedSavingsPct),
		"", "", "",
		"", "", "", "", "", "", "", "", "",
	}
}

func filterPerformanceSuggestionCSV(suggestion filterPerformanceSuggestion, filters filtersEnvelope) []string {
	return []string{
		filterPerformanceDataset, filters.Since, filters.Tool, strconv.FormatBool(filters.Failed), "suggestion",
		suggestion.Tool, suggestion.Filter, suggestion.Case, "", "", "", "",
		strconv.FormatInt(suggestion.Count, 10),
		"", "", "", "", "",
		"", "", "", "",
		"", "", "", fmt.Sprintf("%.2f", suggestion.SavingsPct),
		suggestion.Kind, suggestion.Command, suggestion.Reason,
		"", "", "", "", "", "", "", "", "",
	}
}

func filterPerformanceBuildSummaryCSV(summary metrics.RegistryBuildSummary, filters filtersEnvelope) []string {
	return []string{
		filterPerformanceDataset, filters.Since, filters.Tool, strconv.FormatBool(filters.Failed), "build-summary",
		"", "", "", "", "", "", "",
		"", "", "", "", "", "",
		"", "", "", "",
		"", "", "", "",
		"", "", "",
		strconv.FormatInt(summary.Builds, 10),
		fmt.Sprintf("%.2f", summary.AvgDurationMS),
		strconv.FormatInt(summary.P95DurationMS, 10),
		strconv.FormatInt(summary.MaxDurationMS, 10),
		"", "", "", "", "",
	}
}

func filterPerformanceBuildSourceCSV(row metrics.RegistrySourceBuildRow, filters filtersEnvelope) []string {
	return []string{
		filterPerformanceDataset, filters.Since, filters.Tool, strconv.FormatBool(filters.Failed), "build-source",
		"", "", "", "", "", "", "",
		"", "", "", "", "", "",
		"", "", "", "",
		"", "", "", "",
		"", "", "",
		strconv.FormatInt(row.Builds, 10),
		fmt.Sprintf("%.2f", row.AvgDurationMS),
		strconv.FormatInt(row.P95DurationMS, 10),
		strconv.FormatInt(row.MaxDurationMS, 10),
		row.SourceKind,
		row.SourceDir,
		strconv.FormatInt(row.Errors, 10),
		strconv.FormatInt(row.Definitions, 10),
		strconv.FormatInt(row.Compiled, 10),
	}
}
