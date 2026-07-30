package lifecycle

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fatih/color"

	"github.com/SuppieRK/cmdshape/internal/metrics"
)

const (
	defaultTextLimit          = 15
	textPercentFmt            = "%.1f%%"
	trendInsufficientData     = "insufficient data"
	trendFlatCutoff           = 0.05
	trendFloatEpsilon         = 1e-9
	compactVerdictGrayCutoff  = 10.0
	compactVerdictAmberCutoff = 30.0
)

type labeledLine struct {
	label string
	value string
}

type textTableColumn struct {
	header string
	right  bool
}

var compactGainColors = struct {
	bold         *color.Color
	gray         *color.Color
	verdictGray  *color.Color
	verdictAmber *color.Color
	verdictGreen *color.Color
	trendUp      *color.Color
	trendDown    *color.Color
	trendFlat    *color.Color
}{
	bold:         enabledGainColor(color.Bold),
	gray:         enabledGainColor(color.FgHiBlack),
	verdictGray:  enabledGainColor(color.FgHiBlack),
	verdictAmber: enabledGainColor(color.FgYellow),
	verdictGreen: enabledGainColor(color.FgGreen),
	trendUp:      enabledGainColor(color.FgGreen, color.Bold),
	trendDown:    enabledGainColor(color.FgYellow, color.Bold),
	trendFlat:    enabledGainColor(color.Bold),
}

func enabledGainColor(attrs ...color.Attribute) *color.Color {
	c := color.New(attrs...)
	c.EnableColor()
	return c
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
		parts = append(parts, "nonzero-only")
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
	return fmt.Sprintf("%s cmds · %s source → %s emitted (%s)%s",
		formatInt(total.Commands),
		formatByteSize(total.RawBytes),
		formatByteSize(total.KeptBytes),
		byteChangeText(total.RawBytes, total.KeptBytes),
		compactFilterSuffix(filters, extraTags...),
	)
}

func historyTitle(filters filtersEnvelope, extraTags ...string) string {
	return "cmdshape history" + compactFilterSuffix(filters, extraTags...)
}

func formatPercentText(v float64) string {
	return fmt.Sprintf(textPercentFmt, v)
}

func netReductionBytes(sourceBytes, emittedBytes int64) int64 {
	return sourceBytes - emittedBytes
}

func netReductionPercent(sourceBytes, emittedBytes int64) (float64, bool) {
	if sourceBytes == 0 {
		return 0, emittedBytes == 0
	}
	return float64(netReductionBytes(sourceBytes, emittedBytes)) / float64(sourceBytes) * 100, true
}

func byteChangeText(sourceBytes, emittedBytes int64) string {
	pct, defined := netReductionPercent(sourceBytes, emittedBytes)
	if !defined {
		return "new output"
	}
	switch {
	case pct > 0:
		return formatPercentText(pct) + " net reduction"
	case pct < 0:
		return formatPercentText(-pct) + " expansion"
	default:
		return "no net byte change"
	}
}

func byteChangePercentText(sourceBytes, emittedBytes int64) string {
	pct, defined := netReductionPercent(sourceBytes, emittedBytes)
	if !defined {
		return "new output"
	}
	if pct < 0 {
		return formatPercentText(-pct) + " expansion"
	}
	return formatPercentText(pct)
}

func formatByteSize(v int64) string {
	const unit = int64(1024)
	if v > -unit && v < unit {
		return fmt.Sprintf("%d B", v)
	}

	value := float64(v)
	suffix := "KiB"
	for _, next := range []string{"MiB", "GiB", "TiB"} {
		value /= float64(unit)
		if math.Abs(value) < float64(unit) {
			break
		}
		suffix = next
	}
	formatted := fmt.Sprintf("%.1f", value)
	formatted = strings.TrimSuffix(formatted, ".0")
	return formatted + " " + suffix
}

func summaryReductionText(row metrics.SummaryToolRow) string {
	reduction := netReductionBytes(row.RawBytes, row.KeptBytes)
	return fmt.Sprintf("%s (%s / %s)", row.Tool, formatByteSize(reduction), byteChangePercentText(row.RawBytes, row.KeptBytes))
}

func summaryExpansionText(row metrics.SummaryToolRow) string {
	expansion := row.KeptBytes - row.RawBytes
	return fmt.Sprintf("%s (%s / %s)", row.Tool, formatByteSize(expansion), byteChangePercentText(row.RawBytes, row.KeptBytes))
}

func topReductionText(rows []metrics.SummaryToolRow) string {
	sorted := slices.Clone(rows)
	slices.SortFunc(sorted, func(a, b metrics.SummaryToolRow) int {
		if diff := cmp.Compare(netReductionBytes(b.RawBytes, b.KeptBytes), netReductionBytes(a.RawBytes, a.KeptBytes)); diff != 0 {
			return diff
		}
		if diff := cmp.Compare(b.Commands, a.Commands); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Tool, b.Tool)
	})

	parts := make([]string, 0, 3)
	for _, row := range sorted {
		pct, defined := netReductionPercent(row.RawBytes, row.KeptBytes)
		if !defined || pct <= 20 {
			continue
		}
		parts = append(parts, summaryReductionText(row))
		if len(parts) == 3 {
			break
		}
	}
	return strings.Join(parts, " · ")
}

func lowReductionText(rows []metrics.SummaryToolRow) string {
	sorted := make([]metrics.SummaryToolRow, 0, len(rows))
	for _, row := range rows {
		pct, defined := netReductionPercent(row.RawBytes, row.KeptBytes)
		if defined && pct >= 0 && pct <= 20 {
			sorted = append(sorted, row)
		}
	}
	slices.SortFunc(sorted, func(a, b metrics.SummaryToolRow) int {
		if diff := cmp.Compare(b.Commands, a.Commands); diff != 0 {
			return diff
		}
		aPct, _ := netReductionPercent(a.RawBytes, a.KeptBytes)
		bPct, _ := netReductionPercent(b.RawBytes, b.KeptBytes)
		if diff := cmp.Compare(bPct, aPct); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Tool, b.Tool)
	})

	parts := make([]string, 0, 3)
	for _, row := range sorted {
		parts = append(parts, summaryReductionText(row))
		if len(parts) == 3 {
			break
		}
	}
	return strings.Join(parts, " · ")
}

func expansionText(rows []metrics.SummaryToolRow) string {
	sorted := slices.Clone(rows)
	slices.SortFunc(sorted, func(a, b metrics.SummaryToolRow) int {
		aExpansion := a.KeptBytes - a.RawBytes
		bExpansion := b.KeptBytes - b.RawBytes
		if diff := cmp.Compare(bExpansion, aExpansion); diff != 0 {
			return diff
		}
		if diff := cmp.Compare(b.Commands, a.Commands); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Tool, b.Tool)
	})

	parts := make([]string, 0, 3)
	for _, row := range sorted {
		if row.KeptBytes <= row.RawBytes {
			continue
		}
		parts = append(parts, summaryExpansionText(row))
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
	lines := make([]labeledLine, 0, 3)
	if value := topReductionText(rows); value != "" {
		lines = append(lines, labeledLine{label: "Most net reduction", value: value})
	}
	if value := lowReductionText(rows); value != "" {
		lines = append(lines, labeledLine{label: "Low reduction", value: value})
	}
	if value := expansionText(rows); value != "" {
		lines = append(lines, labeledLine{label: "Expansion", value: value})
	}
	return lines
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

func trendSummaryText(rows []metrics.PeriodRow, period string) string {
	if len(rows) < 2 {
		return trendInsufficientData
	}
	sorted := slices.Clone(rows)
	slices.SortFunc(sorted, func(a, b metrics.PeriodRow) int {
		return cmp.Compare(a.BucketStart, b.BucketStart)
	})
	if len(sorted) == 2 {
		earlier, earlierDefined := periodNetReductionPercent(sorted[:1])
		recent, recentDefined := periodNetReductionPercent(sorted[1:])
		if !earlierDefined || !recentDefined {
			return trendInsufficientData
		}
		return formatTrendSummary(earlier, recent, period)
	}
	split := trendSplitIndex(len(sorted), period)
	if split <= 0 || split >= len(sorted) {
		return trendInsufficientData
	}

	earlier, earlierDefined := periodNetReductionPercent(sorted[:split])
	recent, recentDefined := periodNetReductionPercent(sorted[split:])
	if !earlierDefined || !recentDefined {
		return trendInsufficientData
	}
	return formatTrendSummary(earlier, recent, period)
}

func periodNetReductionPercent(rows []metrics.PeriodRow) (float64, bool) {
	var sourceBytes int64
	var emittedBytes int64
	for _, row := range rows {
		sourceBytes += row.RawBytes
		emittedBytes += row.KeptBytes
	}
	return netReductionPercent(sourceBytes, emittedBytes)
}

func formatTrendSummary(earlier, recent float64, period string) string {
	diff := recent - earlier
	label := trendLabel(period)

	switch {
	case diff > trendFlatCutoff+trendFloatEpsilon:
		return fmt.Sprintf("↑ +%.1f pts %s (%s → %s)",
			diff,
			label,
			formatPercentText(earlier),
			formatPercentText(recent),
		)
	case diff < -trendFlatCutoff-trendFloatEpsilon:
		return fmt.Sprintf("↓ -%.1f pts %s (%s → %s)",
			-diff,
			label,
			formatPercentText(earlier),
			formatPercentText(recent),
		)
	default:
		return fmt.Sprintf("→ flat %s (%s → %s)",
			label,
			formatPercentText(earlier),
			formatPercentText(recent),
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

func compactGainVerdictColor(pct float64) *color.Color {
	switch {
	case pct < compactVerdictGrayCutoff:
		return compactGainColors.verdictGray
	case pct < compactVerdictAmberCutoff:
		return compactGainColors.verdictAmber
	default:
		return compactGainColors.verdictGreen
	}
}

func styleCompactGainHeadline(total metrics.SummaryTotal, filters filtersEnvelope, extraTags ...string) string {
	reductionPercent, defined := netReductionPercent(total.RawBytes, total.KeptBytes)
	verdictColor := compactGainColors.verdictAmber
	if defined && reductionPercent >= 0 {
		verdictColor = compactGainVerdictColor(reductionPercent)
	}
	verdict := verdictColor.Sprint(byteChangeText(total.RawBytes, total.KeptBytes))
	return fmt.Sprintf("%s cmds · %s source → %s emitted (%s)%s",
		compactGainColors.bold.Sprint(formatInt(total.Commands)),
		compactGainColors.bold.Sprint(formatByteSize(total.RawBytes)),
		compactGainColors.bold.Sprint(formatByteSize(total.KeptBytes)),
		verdict,
		compactFilterSuffix(filters, extraTags...),
	)
}

func styleCompactGainLabel(label string, width int) string {
	return compactGainColors.bold.Sprint(fmt.Sprintf("%-*s", width, label))
}

func styleCompactGainMetricValue(value string) string {
	parts := strings.Split(value, " · ")
	for i, part := range parts {
		tool, rest, ok := strings.Cut(part, " (")
		if !ok {
			continue
		}
		inner := strings.TrimSuffix(rest, ")")
		bytes, pct, ok := strings.Cut(inner, " / ")
		if !ok {
			continue
		}
		parts[i] = compactGainColors.bold.Sprint(tool) +
			" " + compactGainColors.gray.Sprint("(") +
			compactGainColors.gray.Sprint(bytes) +
			compactGainColors.gray.Sprint(" / ") +
			compactGainColors.gray.Sprint(pct) +
			compactGainColors.gray.Sprint(")")
	}
	return strings.Join(parts, " · ")
}

func styleCompactTrendDelta(prefix string) string {
	switch {
	case strings.HasPrefix(prefix, "↑"):
		return compactGainColors.trendUp.Sprint(prefix)
	case strings.HasPrefix(prefix, "↓"):
		return compactGainColors.trendDown.Sprint(prefix)
	default:
		return compactGainColors.trendFlat.Sprint(prefix)
	}
}

func styleCompactGainTrendValue(value string) string {
	if value == trendInsufficientData {
		return value
	}
	for _, label := range []string{"day over day", "week over week", "month over month"} {
		prefix, rest, ok := strings.Cut(value, " "+label+" ")
		if !ok {
			continue
		}
		return styleCompactTrendDelta(prefix) + " " + label + " " + compactGainColors.gray.Sprint(rest)
	}
	return value
}

func styleCompactGainLines(lines []labeledLine) []labeledLine {
	styled := make([]labeledLine, 0, len(lines))
	for _, line := range lines {
		switch line.label {
		case "Most net reduction", "Low reduction", "Expansion":
			line.value = styleCompactGainMetricValue(line.value)
		case "Trend":
			line.value = styleCompactGainTrendValue(line.value)
		}
		styled = append(styled, line)
	}
	return styled
}

func printCompactGainLines(lines []labeledLine) {
	if len(lines) == 0 {
		return
	}
	width := 0
	for _, line := range lines {
		width = max(width, len(line.label))
	}
	for _, line := range lines {
		fmt.Printf("%s : %s\n", styleCompactGainLabel(line.label, width), line.value)
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

	fmt.Println(styleCompactGainHeadline(total, filters, extraTags...))
	fmt.Println()
	if total.Commands == 0 {
		fmt.Println(noResultsMsg)
		return nil
	}

	lines := summaryInsightLines(rows)
	lines = append(lines, labeledLine{label: "Trend", value: trendSummaryText(trendRows, trendPeriod)})
	printCompactGainLines(styleCompactGainLines(lines))
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
	sorted := slices.Clone(rows)
	slices.SortFunc(sorted, func(a, b metrics.SummaryToolRow) int {
		if diff := cmp.Compare(netReductionBytes(b.RawBytes, b.KeptBytes), netReductionBytes(a.RawBytes, a.KeptBytes)); diff != 0 {
			return diff
		}
		if diff := cmp.Compare(b.Commands, a.Commands); diff != 0 {
			return diff
		}
		return cmp.Compare(a.Tool, b.Tool)
	})
	return sorted
}

func padTableCell(value string, width int, right bool) string {
	padding := max(width-utf8.RuneCountInString(value), 0)
	if right {
		return " " + strings.Repeat(" ", padding) + value + " "
	}
	return " " + value + strings.Repeat(" ", padding) + " "
}

func renderTextTable(columns []textTableColumn, rows [][]string) string {
	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = utf8.RuneCountInString(col.header)
	}
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
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
			formatInt(row.RawBytes),
			formatInt(row.KeptBytes),
			formatInt(netReductionBytes(row.RawBytes, row.KeptBytes)),
			byteChangePercentText(row.RawBytes, row.KeptBytes),
		})
	}

	fmt.Print(renderTextTable([]textTableColumn{
		{header: "TOOL"},
		{header: "COUNT", right: true},
		{header: "SOURCE", right: true},
		{header: "EMITTED", right: true},
		{header: "NET REDUCTION", right: true},
		{header: "REDUCTION %", right: true},
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
			byteChangePercentText(row.RawBytes, row.KeptBytes),
		})
	}

	fmt.Print(renderTextTable([]textTableColumn{
		{header: "TIMESTAMP"},
		{header: "COMMAND"},
		{header: "STATUS"},
		{header: "REDUCTION %", right: true},
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
			byteChangePercentText(row.RawBytes, row.KeptBytes),
		})
	}

	fmt.Print(renderTextTable([]textTableColumn{
		{header: "TIMESTAMP"},
		{header: "SOURCE"},
		{header: "COMMAND"},
		{header: "STATUS"},
		{header: "REDUCTION %", right: true},
	}, tableRows))
	return nil
}
