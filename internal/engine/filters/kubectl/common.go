package kubectlfilters

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

const (
	kubectlOutputLongFlag   = "--output"
	kubectlOutputEqualsFlag = "--output="
)

type tableCompactorConfig struct {
	tool            string
	resourceLabel   string
	headerOptions   [][]string
	namespaceAware  bool
	healthEvaluator func(row tableRow) (healthy bool, anomalyReason string)
	signature       func(row tableRow) string
}

type tableCompactorFilter struct {
	cfg tableCompactorConfig
}

type tableRow struct {
	headerIndex map[string]int
	fields      []string
}

func (r tableRow) get(header string) string {
	idx, ok := r.headerIndex[header]
	if !ok || idx < 0 || idx >= len(r.fields) {
		return ""
	}
	return r.fields[idx]
}

func (f tableCompactorFilter) Tool() string      { return f.cfg.tool }
func (f tableCompactorFilter) Aliases() []string { return nil }
func (f tableCompactorFilter) MaskingHorizon() int {
	return 0
}
func (f tableCompactorFilter) Prepare(args []string) engine.PrepareResult {
	if !allowlistedGetArgs(args) {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}
	return engine.PrepareResult{NormalizedArgs: args}
}
func (f tableCompactorFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}

func (f tableCompactorFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream == engine.StderrStream {
		return f.processTableCompactorStderr(ev)
	}
	if ev.Type != engine.EventEOF {
		return engine.Decision{Action: engine.ActionCollect}
	}
	return f.processTableCompactorEOF(mem)
}

func (f tableCompactorFilter) processTableCompactorStderr(ev engine.Event) engine.Decision {
	if ev.Type != engine.EventLine {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	if strings.Contains(strings.ToLower(ev.Line), "no resources found") {
		return engine.Decision{Action: engine.ActionImmediate, Output: fmt.Sprintf("No %s found\n", f.cfg.resourceLabel)}
	}
	return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
}

func (f tableCompactorFilter) processTableCompactorEOF(mem *engine.OrderedSetBuffer) engine.Decision {
	raw := strings.TrimRight(mem.Joined(), "\n")
	if strings.TrimSpace(raw) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	lines := filtercommon.NonEmptyLines(raw)
	if len(lines) == 0 {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	header := lines[0]
	headFields := strings.Fields(header)
	if !matchesKnownHeader(headFields, f.cfg.headerOptions) {
		return engine.Decision{Action: engine.ActionFlush, Output: raw + "\n"}
	}
	headerIndex := make(map[string]int, len(headFields))
	for i, h := range headFields {
		headerIndex[h] = i
	}
	rows := lines[1:]
	if len(rows) == 0 {
		return engine.Decision{Action: engine.ActionFlush, Output: header + "\n"}
	}

	aggregates := f.collectTableRowStats(rows, headFields, headerIndex)

	var out strings.Builder
	out.WriteString(header)
	out.WriteString("\n")

	for _, a := range aggregates.anomalies {
		out.WriteString(a)
		out.WriteString("\n")
	}

	f.appendNamespaceSummary(&out, aggregates.nsTotal, aggregates.nsHealthy)
	f.appendGroupedSummary(&out, aggregates.groups)

	return engine.Decision{Action: engine.ActionFlush, Output: out.String()}
}

type tableRowStats struct {
	groups    map[string]int
	nsHealthy map[string]int
	nsTotal   map[string]int
	anomalies []string
}

func (f tableCompactorFilter) collectTableRowStats(rows []string, headFields []string, headerIndex map[string]int) tableRowStats {
	stats := tableRowStats{
		groups:    map[string]int{},
		nsHealthy: map[string]int{},
		nsTotal:   map[string]int{},
		anomalies: make([]string, 0),
	}
	for _, rowLine := range rows {
		f.collectTableRowLineStats(rowLine, headFields, headerIndex, &stats)
	}
	return stats
}

func (f tableCompactorFilter) collectTableRowLineStats(rowLine string, headFields []string, headerIndex map[string]int, stats *tableRowStats) {
	fields := strings.Fields(rowLine)
	if len(fields) < len(headFields) {
		stats.anomalies = append(stats.anomalies, rowLine)
		return
	}

	row := tableRow{headerIndex: headerIndex, fields: fields}
	ns := ""
	if f.cfg.namespaceAware {
		ns = row.get("NAMESPACE")
		if ns != "" {
			stats.nsTotal[ns]++
		}
	}

	healthy, _ := f.cfg.healthEvaluator(row)
	if !healthy {
		stats.anomalies = append(stats.anomalies, rowLine)
		return
	}

	if f.cfg.namespaceAware && ns != "" {
		stats.nsHealthy[ns]++
		return
	}

	sig := f.cfg.signature(row)
	if sig == "" {
		sig = rowLine
	}
	stats.groups[sig]++
}

func (f tableCompactorFilter) appendNamespaceSummary(out *strings.Builder, nsTotal map[string]int, nsHealthy map[string]int) {
	if len(nsTotal) == 0 {
		return
	}
	ns := make([]string, 0, len(nsTotal))
	for k := range nsTotal {
		ns = append(ns, k)
	}
	sort.Strings(ns)
	for _, n := range ns {
		total := nsTotal[n]
		healthy := nsHealthy[n]
		unhealthy := total - healthy
		if unhealthy <= 0 {
			_, _ = fmt.Fprintf(out, "%s: [%d %s: all Running]\n", n, total, f.cfg.resourceLabel)
			continue
		}
		_, _ = fmt.Fprintf(out, "%s: [%d %s: %d healthy, %d unhealthy]\n", n, total, f.cfg.resourceLabel, healthy, unhealthy)
	}
}

func (f tableCompactorFilter) appendGroupedSummary(out *strings.Builder, groups map[string]int) {
	if len(groups) == 0 {
		return
	}
	sigs := make([]string, 0, len(groups))
	for s := range groups {
		sigs = append(sigs, s)
	}
	sort.Strings(sigs)
	for _, s := range sigs {
		_, _ = fmt.Fprintf(out, "[%d] %s\n", groups[s], s)
	}
}

func matchesKnownHeader(actual []string, options [][]string) bool {
	for _, opt := range options {
		if len(actual) < len(opt) {
			continue
		}
		ok := true
		for i := range opt {
			if !strings.EqualFold(actual[i], opt[i]) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func allowlistedGetArgs(args []string) bool {
	if len(args) == 0 {
		return true
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		allow, consumesNext := isAllowlistedGetArg(arg, i+1 < len(args))
		if consumesNext {
			i++
		}
		if !allow {
			return false
		}
	}
	// explicit output only allow wide here; structured handled by parent.
	if v := outputValue(args); v != "" && v != "wide" {
		return false
	}
	return true
}

func isAllowlistedGetArg(arg string, hasNext bool) (allow bool, consumesNext bool) {
	switch arg {
	case "-n", "--namespace", "-o", kubectlOutputLongFlag, "-l", "--selector", "--field-selector":
		if hasNext {
			return true, true
		}
		return false, false
	case "-A", "--all-namespaces", "-w", "--watch", "--no-headers":
		return true, false
	}
	if strings.HasPrefix(arg, kubectlOutputEqualsFlag) {
		v := strings.TrimPrefix(arg, kubectlOutputEqualsFlag)
		return v == "wide", false
	}
	if strings.HasPrefix(arg, "-") {
		return false, false
	}
	return true, false
}

func outputValue(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := strings.ToLower(strings.TrimSpace(args[i]))
		if (arg == "-o" || arg == kubectlOutputLongFlag) && i+1 < len(args) {
			return strings.ToLower(strings.TrimSpace(args[i+1]))
		}
		if strings.HasPrefix(arg, kubectlOutputEqualsFlag) {
			return strings.TrimPrefix(arg, kubectlOutputEqualsFlag)
		}
	}
	return ""
}

func hasOutputValue(args []string) bool {
	return outputValue(args) != ""
}

func isPodsHealthy(row tableRow) (bool, string) {
	status := strings.ToLower(row.get("STATUS"))
	ready := row.get("READY")
	restarts := row.get("RESTARTS")
	if strings.Contains(status, "error") || strings.Contains(status, "crash") {
		return false, "status"
	}
	if strings.HasPrefix(ready, "0/") {
		return false, "ready"
	}
	if restarts != "" {
		n, err := strconv.Atoi(restarts)
		if err != nil || n > 0 {
			return false, "restarts"
		}
	}
	return strings.EqualFold(status, "running") || strings.EqualFold(status, "completed"), ""
}

func isNodesHealthy(row tableRow) (bool, string) {
	status := strings.ToLower(row.get("STATUS"))
	if strings.Contains(status, "notready") || strings.Contains(status, "unknown") {
		return false, "status"
	}
	return strings.Contains(status, "ready"), ""
}

func isServicesHealthy() (bool, string) {
	return true, ""
}

func formatResourceSignature(resource string, parts []string) string {
	return resource + ": " + strings.TrimSpace(strings.Join(parts, " "))
}
