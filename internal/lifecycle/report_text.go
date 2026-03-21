package lifecycle

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"go-command-compression-proxy/internal/metrics"
)

const (
	defaultTextLimit = 15
	textPercentFmt   = "%.1f%%"
)

type labeledLine struct {
	label string
	value string
}

type textTableColumn struct {
	header string
	right  bool
}

func defaultGainTrendPeriod(period string) string {
	if strings.TrimSpace(period) != "" {
		return period
	}
	return "week"
}

func compactFilterSuffix(filters filtersEnvelope, extraTags ...string) string {
	parts := make([]string, 0, 4+len(extraTags))
	if filters.Since != "" {
		parts = append(parts, "since="+filters.Since)
	}
	if filters.Tool != "" {
		parts = append(parts, "tool="+filters.Tool)
	}
	if filters.Failed {
		parts = append(parts, "failed-only")
	}
	for _, tag := range extraTags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		parts = append(parts, tag)
	}
	if len(parts) == 0 {
		return ""
	}
	return " [" + strings.Join(parts, " ") + "]"
}

func gainHeadline(total metrics.SummaryTotal, filters filtersEnvelope, extraTags ...string) string {
	return fmt.Sprintf("%s cmds · %s → %s tokens (%s saved)%s",
		formatInt(total.Commands),
		formatInt(total.EstimatedInputTokens),
		formatInt(total.EstimatedOutputTokens),
		formatPercentText(total.EstimatedSavingsPct),
		compactFilterSuffix(filters, extraTags...),
	)
}

func historyTitle(filters filtersEnvelope, extraTags ...string) string {
	return "ccp history" + compactFilterSuffix(filters, extraTags...)
}

func formatPercentText(v float64) string {
	return fmt.Sprintf(textPercentFmt, v)
}

func summaryPercentText(row metrics.SummaryToolRow) string {
	return fmt.Sprintf("%s %s", row.Tool, formatPercentText(row.EstimatedSavingsPct))
}

func summaryCountText(row metrics.SummaryToolRow) string {
	return fmt.Sprintf("%s (%s cmds)", row.Tool, formatInt(row.Commands))
}

func shouldSplitWinsDrags(rows []metrics.SummaryToolRow) bool {
	if len(rows) < 2 {
		return false
	}

	minPct := rows[0].EstimatedSavingsPct
	maxPct := rows[0].EstimatedSavingsPct
	hasStrong := false
	hasWeak := false

	for _, row := range rows {
		pct := row.EstimatedSavingsPct
		minPct = min(minPct, pct)
		maxPct = max(maxPct, pct)
		if pct >= 35 {
			hasStrong = true
		}
		if pct <= 20 {
			hasWeak = true
		}
	}

	spread := maxPct - minPct
	if len(rows) == 2 {
		return hasStrong && hasWeak && spread >= 20
	}
	return len(rows) >= 3 && hasStrong && hasWeak && spread >= 10
}

func topWinsText(rows []metrics.SummaryToolRow) string {
	sorted := append([]metrics.SummaryToolRow(nil), rows...)
	slices.SortFunc(sorted, func(a, b metrics.SummaryToolRow) int {
		if diff := cmp.Compare(b.EstimatedSavedTokens, a.EstimatedSavedTokens); diff != 0 {
			return diff
		}
		if diff := cmp.Compare(b.Commands, a.Commands); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Tool, b.Tool)
	})

	parts := make([]string, 0, 3)
	for _, row := range sorted {
		if row.EstimatedSavedTokens <= 0 {
			continue
		}
		parts = append(parts, summaryPercentText(row))
		if len(parts) == 3 {
			break
		}
	}
	return strings.Join(parts, " · ")
}

func dragToolsText(rows []metrics.SummaryToolRow) string {
	sorted := make([]metrics.SummaryToolRow, 0, len(rows))
	for _, row := range rows {
		if row.EstimatedSavingsPct <= 20 {
			sorted = append(sorted, row)
		}
	}
	if len(sorted) == 0 {
		sorted = append(sorted, rows...)
	}
	slices.SortFunc(sorted, func(a, b metrics.SummaryToolRow) int {
		if diff := cmp.Compare(b.Commands, a.Commands); diff != 0 {
			return diff
		}
		if diff := cmp.Compare(a.EstimatedSavingsPct, b.EstimatedSavingsPct); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Tool, b.Tool)
	})

	parts := make([]string, 0, 3)
	for _, row := range sorted {
		parts = append(parts, summaryCountText(row))
		if len(parts) == 3 {
			break
		}
	}
	return strings.Join(parts, " · ")
}

func toolsSummaryText(rows []metrics.SummaryToolRow) string {
	sorted := append([]metrics.SummaryToolRow(nil), rows...)
	slices.SortFunc(sorted, func(a, b metrics.SummaryToolRow) int {
		if diff := cmp.Compare(b.EstimatedSavedTokens, a.EstimatedSavedTokens); diff != 0 {
			return diff
		}
		if diff := cmp.Compare(b.Commands, a.Commands); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Tool, b.Tool)
	})

	parts := make([]string, 0, 3)
	for _, row := range sorted {
		parts = append(parts, summaryPercentText(row))
		if len(parts) == 3 {
			break
		}
	}
	return strings.Join(parts, " · ")
}

func summaryInsightLines(rows []metrics.SummaryToolRow) []labeledLine {
	if len(rows) == 0 {
		return nil
	}
	if shouldSplitWinsDrags(rows) {
		wins := topWinsText(rows)
		drag := dragToolsText(rows)
		if wins != "" && drag != "" {
			return []labeledLine{
				{label: "Wins", value: wins},
				{label: "Drag", value: drag},
			}
		}
	}
	return []labeledLine{{label: "Tools", value: toolsSummaryText(rows)}}
}

func trendLabel(period string) string {
	switch period {
	case "day":
		return "day over day"
	case "month":
		return "month over month"
	default:
		return "week over week"
	}
}

func trendSavingsBucket(pct float64) string {
	switch {
	case pct <= 0:
		return "none"
	case pct < 20:
		return "low"
	case pct < 40:
		return "mid"
	case pct < 60:
		return "high"
	default:
		return "extreme"
	}
}

func trendDeltaBucket(diff float64) string {
	absDiff := math.Abs(diff)
	switch {
	case absDiff < 2:
		return "small"
	case absDiff < 6:
		return "medium"
	default:
		return "large"
	}
}

func deterministicVariant(variants []string, earlier, recent float64) string {
	if len(variants) == 0 {
		return ""
	}
	seed := int64(math.Round(earlier*10))*31 + int64(math.Round(recent*10))*17
	idx := int(seed % int64(len(variants)))
	if idx < 0 {
		idx += len(variants)
	}
	return variants[idx]
}

func trendSuffixUp(deltaBucket, savingsBucket string, earlier, recent float64) string {
	switch deltaBucket {
	case "small":
		return deterministicVariant([]string{"uptick", "firming"}, earlier, recent)
	case "medium":
		if savingsBucket == "high" || savingsBucket == "extreme" {
			return deterministicVariant([]string{"gaining", "dialed in"}, earlier, recent)
		}
		return deterministicVariant([]string{"gaining", "coming along"}, earlier, recent)
	default:
		if savingsBucket == "high" || savingsBucket == "extreme" {
			return deterministicVariant([]string{"clear gain", "on a roll"}, earlier, recent)
		}
		return deterministicVariant([]string{"clear gain", "picking up"}, earlier, recent)
	}
}

func trendSuffixDown(deltaBucket, savingsBucket string, earlier, recent float64) string {
	if savingsBucket == "none" || savingsBucket == "low" {
		switch deltaBucket {
		case "small":
			return deterministicVariant([]string{"thin", "fading"}, earlier, recent)
		case "medium":
			return deterministicVariant([]string{"dragging", "slipping"}, earlier, recent)
		default:
			return deterministicVariant([]string{"backslide", "off track"}, earlier, recent)
		}
	}

	switch deltaBucket {
	case "small":
		return deterministicVariant([]string{"easing", "soft dip"}, earlier, recent)
	case "medium":
		return deterministicVariant([]string{"losing ground", "cooling off"}, earlier, recent)
	default:
		return deterministicVariant([]string{"hard fade", "running cold"}, earlier, recent)
	}
}

func trendSuffixFlat(savingsBucket string, earlier, recent float64) string {
	switch savingsBucket {
	case "none", "low":
		return deterministicVariant([]string{"stuck low", "flatline"}, earlier, recent)
	case "high", "extreme":
		return deterministicVariant([]string{"holding high", "dialed in"}, earlier, recent)
	default:
		return deterministicVariant([]string{"holding", "steady"}, earlier, recent)
	}
}

func trendSuffix(earlier, recent float64) string {
	diff := recent - earlier
	deltaBucket := trendDeltaBucket(diff)
	savingsBucket := trendSavingsBucket(recent)

	switch {
	case diff > 0.05:
		return trendSuffixUp(deltaBucket, savingsBucket, earlier, recent)
	case diff < -0.05:
		return trendSuffixDown(deltaBucket, savingsBucket, earlier, recent)
	default:
		return trendSuffixFlat(savingsBucket, earlier, recent)
	}
}

func trendSummaryText(rows []metrics.PeriodRow, period string) string {
	if len(rows) < 2 {
		return "insufficient data"
	}
	sorted := append([]metrics.PeriodRow(nil), rows...)
	slices.SortFunc(sorted, func(a, b metrics.PeriodRow) int {
		return cmp.Compare(a.BucketStart, b.BucketStart)
	})
	split := trendSplitIndex(len(sorted), period)
	if split <= 0 || split >= len(sorted) {
		return "insufficient data"
	}

	earlier := averageSavings(sorted[:split])
	recent := averageSavings(sorted[split:])
	diff := recent - earlier
	label := trendLabel(period)

	switch {
	case diff > 0.05:
		return fmt.Sprintf("↑ +%.1f pts %s (%s → %s) · %s",
			diff,
			label,
			formatPercentText(earlier),
			formatPercentText(recent),
			trendSuffix(earlier, recent),
		)
	case diff < -0.05:
		return fmt.Sprintf("↓ -%.1f pts %s (%s → %s) · %s",
			-diff,
			label,
			formatPercentText(earlier),
			formatPercentText(recent),
			trendSuffix(earlier, recent),
		)
	default:
		return fmt.Sprintf("→ flat %s (%s → %s) · %s",
			label,
			formatPercentText(earlier),
			formatPercentText(recent),
			trendSuffix(earlier, recent),
		)
	}
}

func printLabeledLines(lines []labeledLine) {
	if len(lines) == 0 {
		return
	}
	width := 0
	for _, line := range lines {
		width = max(width, len(line.label))
	}
	for _, line := range lines {
		fmt.Printf("%-*s : %s\n", width, line.label, line.value)
	}
}

func printCompactGainSummary(filters filtersEnvelope, total metrics.SummaryTotal, rows []metrics.SummaryToolRow, trendRows []metrics.PeriodRow, trendPeriod, requestedPeriod string, global bool) error {
	extraTags := make([]string, 0, 2)
	if requestedPeriod != "" {
		extraTags = append(extraTags, "period="+requestedPeriod)
	}
	if global {
		extraTags = append(extraTags, "global")
	}

	fmt.Println(gainHeadline(total, filters, extraTags...))
	fmt.Println()
	if total.Commands == 0 {
		fmt.Println(noResultsMsg)
		return nil
	}

	lines := summaryInsightLines(rows)
	lines = append(lines, labeledLine{label: "Trend", value: trendSummaryText(trendRows, trendPeriod)})
	printLabeledLines(lines)
	return nil
}

func resolvedTextLimit(limit int) int {
	if limit < 0 {
		return defaultTextLimit
	}
	return limit
}

func limitRows[T any](rows []T, limit int) ([]T, int) {
	total := len(rows)
	resolved := resolvedTextLimit(limit)
	if resolved == 0 || total <= resolved {
		return rows, total
	}
	return rows[:resolved], total
}

func tableSummaryLine(displayed, total int, noun string) string {
	if displayed < total {
		return fmt.Sprintf("showing %d of %d %s, use --limit N to see more", displayed, total, noun)
	}
	return fmt.Sprintf("showing %d of %d %s", displayed, total, noun)
}

func sortGainTableRows(rows []metrics.SummaryToolRow) []metrics.SummaryToolRow {
	savingRows := make([]metrics.SummaryToolRow, 0, len(rows))
	zeroRows := make([]metrics.SummaryToolRow, 0, len(rows))
	for _, row := range rows {
		if row.EstimatedSavedTokens == 0 {
			zeroRows = append(zeroRows, row)
			continue
		}
		savingRows = append(savingRows, row)
	}

	slices.SortFunc(savingRows, func(a, b metrics.SummaryToolRow) int {
		if diff := cmp.Compare(b.EstimatedSavedTokens, a.EstimatedSavedTokens); diff != 0 {
			return diff
		}
		if diff := cmp.Compare(b.Commands, a.Commands); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Tool, b.Tool)
	})
	slices.SortFunc(zeroRows, func(a, b metrics.SummaryToolRow) int {
		if diff := cmp.Compare(b.Commands, a.Commands); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Tool, b.Tool)
	})
	return append(savingRows, zeroRows...)
}

func runeLen(input string) int {
	return utf8.RuneCountInString(input)
}

func padTableCell(value string, width int, right bool) string {
	padding := width - runeLen(value)
	if padding < 0 {
		padding = 0
	}
	if right {
		return " " + strings.Repeat(" ", padding) + value + " "
	}
	return " " + value + strings.Repeat(" ", padding) + " "
}

func renderTextTable(columns []textTableColumn, rows [][]string) string {
	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = runeLen(col.header)
	}
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], runeLen(cell))
		}
	}

	var b strings.Builder
	writeBorder := func(left, mid, right string) {
		b.WriteString(left)
		for i, width := range widths {
			b.WriteString(strings.Repeat("-", width+2))
			if i == len(widths)-1 {
				b.WriteString(right)
			} else {
				b.WriteString(mid)
			}
		}
		b.WriteByte('\n')
	}

	writeRow := func(values []string) {
		b.WriteString("|")
		for i, value := range values {
			b.WriteString(padTableCell(value, widths[i], columns[i].right))
			b.WriteString("|")
		}
		b.WriteByte('\n')
	}

	writeBorder("+", "+", "+")
	headers := make([]string, len(columns))
	for i, col := range columns {
		headers[i] = col.header
	}
	writeRow(headers)
	writeBorder("+", "+", "+")
	for _, row := range rows {
		writeRow(row)
	}
	writeBorder("+", "+", "+")
	return b.String()
}

func printSummaryTableText(filters filtersEnvelope, total metrics.SummaryTotal, rows []metrics.SummaryToolRow, limit int, requestedPeriod string, global bool) error {
	extraTags := make([]string, 0, 2)
	if requestedPeriod != "" {
		extraTags = append(extraTags, "period="+requestedPeriod)
	}
	if global {
		extraTags = append(extraTags, "global")
	}

	fmt.Println(gainHeadline(total, filters, extraTags...))
	fmt.Println()
	if total.Commands == 0 || len(rows) == 0 {
		fmt.Println(noResultsMsg)
		return nil
	}

	sortedRows := sortGainTableRows(rows)
	limitedRows, totalRows := limitRows(sortedRows, limit)
	fmt.Println(tableSummaryLine(len(limitedRows), totalRows, "tools"))
	fmt.Println()

	tableRows := make([][]string, 0, len(limitedRows))
	for _, row := range limitedRows {
		tableRows = append(tableRows, []string{
			row.Tool,
			formatInt(row.Commands),
			formatInt(row.EstimatedInputTokens),
			formatInt(row.EstimatedOutputTokens),
			formatInt(row.EstimatedSavedTokens),
			formatPercentText(row.EstimatedSavingsPct),
		})
	}

	fmt.Print(renderTextTable([]textTableColumn{
		{header: "TOOL"},
		{header: "COUNT", right: true},
		{header: "NATIVE", right: true},
		{header: "PROXIED", right: true},
		{header: "SAVED", right: true},
		{header: "SAVINGS", right: true},
	}, tableRows))
	return nil
}

func printHistoryTableTitle(filters filtersEnvelope, global bool) {
	if global {
		fmt.Println(historyTitle(filters, "global"))
		return
	}
	fmt.Println(historyTitle(filters))
}

func printHistoryTable(rows []metrics.HistoryRow, filters filtersEnvelope, limit int) error {
	printHistoryTableTitle(filters, false)
	fmt.Println()
	if len(rows) == 0 {
		fmt.Println(noResultsMsg)
		return nil
	}

	limitedRows, totalRows := limitRows(rows, limit)
	fmt.Println(tableSummaryLine(len(limitedRows), totalRows, "rows"))
	fmt.Println()

	tableRows := make([][]string, 0, len(limitedRows))
	for _, row := range limitedRows {
		tableRows = append(tableRows, []string{
			row.Timestamp.Format(time.RFC3339),
			truncateForDisplay(row.Command, 36),
			historyStatus(row),
			formatPercentText(row.EstimatedSavingsPct),
		})
	}

	fmt.Print(renderTextTable([]textTableColumn{
		{header: "TIMESTAMP"},
		{header: "COMMAND"},
		{header: "STATUS"},
		{header: "SAVINGS", right: true},
	}, tableRows))
	return nil
}

func printGlobalHistoryTable(rows []globalHistoryRow, filters filtersEnvelope, limit int) error {
	printHistoryTableTitle(filters, true)
	fmt.Println()
	if len(rows) == 0 {
		fmt.Println(noResultsMsg)
		return nil
	}

	limitedRows, totalRows := limitRows(rows, limit)
	fmt.Println(tableSummaryLine(len(limitedRows), totalRows, "rows"))
	fmt.Println()

	tableRows := make([][]string, 0, len(limitedRows))
	for _, row := range limitedRows {
		tableRows = append(tableRows, []string{
			row.Timestamp.Format(time.RFC3339),
			truncateTailForDisplay(row.Source, 28),
			truncateForDisplay(row.Command, 36),
			historyStatus(row.HistoryRow),
			formatPercentText(row.EstimatedSavingsPct),
		})
	}

	fmt.Print(renderTextTable([]textTableColumn{
		{header: "TIMESTAMP"},
		{header: "SOURCE"},
		{header: "COMMAND"},
		{header: "STATUS"},
		{header: "SAVINGS", right: true},
	}, tableRows))
	return nil
}
