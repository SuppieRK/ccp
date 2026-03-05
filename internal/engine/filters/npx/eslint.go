package npxfilters

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"go-command-compression-proxy/internal/engine"
)

func NewNpxEslintFilter() engine.ToolFilter { return eslintFilter{} }

type eslintFilter struct{}

func (eslintFilter) Tool() string      { return "npx eslint" }
func (eslintFilter) Aliases() []string { return nil }
func (eslintFilter) MaskingHorizon() int {
	return 0
}
func (eslintFilter) Prepare(args []string) engine.PrepareResult {
	normalized := append([]string{}, args...)
	if !hasESLintFormatArg(normalized) {
		normalized = append(normalized, "-f", "json")
	}
	return engine.PrepareResult{NormalizedArgs: normalized}
}
func (eslintFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}
func (eslintFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream == engine.StderrStream {
		switch ev.Type {
		case engine.EventLine:
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		default:
			return engine.Decision{Action: engine.ActionIgnore}
		}
	}
	switch ev.Type {
	case engine.EventTick, engine.EventLine:
		return engine.Decision{Action: engine.ActionCollect}
	case engine.EventEOF:
		return engine.Decision{Action: engine.ActionIgnore}
	case engine.EventExit:
		raw := mem.Joined()
		if strings.TrimSpace(raw) == "" {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		out := stripNpxWrapperNoise(raw)
		if strings.TrimSpace(out) == "" {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		if summary, ok := summarizeESLintOutput(out); ok {
			if summary == "" {
				return engine.Decision{Action: engine.ActionIgnore}
			}
			return engine.Decision{Action: engine.ActionFlush, Output: summary}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: out}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

type eslintMessage struct {
	RuleID   string `json:"ruleId"`
	Severity int    `json:"severity"`
	Message  string `json:"message"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
}

type eslintResult struct {
	FilePath     string          `json:"filePath"`
	Messages     []eslintMessage `json:"messages"`
	ErrorCount   int             `json:"errorCount"`
	WarningCount int             `json:"warningCount"`
}

type eslintFileSummary struct {
	path      string
	issues    int
	errorCnt  int
	warnCnt   int
	messages  []eslintMessage
	shortPath string
}

type eslintRuleCount struct {
	rule  string
	count int
}

type eslintSummaryData struct {
	totalErrors     int
	totalWarnings   int
	filesWithIssues int
	files           []eslintFileSummary
	rules           []eslintRuleCount
}

func hasESLintFormatArg(args []string) bool {
	for _, arg := range args {
		a := strings.TrimSpace(strings.ToLower(arg))
		if a == "-f" || a == "--format" || strings.HasPrefix(a, "--format=") {
			return true
		}
	}
	return false
}

func summarizeESLintOutput(raw string) (string, bool) {
	results, ok := parseESLintResults(raw)
	if !ok {
		return "", false
	}
	summary := buildESLintSummaryData(results)
	if summary.totalErrors == 0 && summary.totalWarnings == 0 {
		return "", true
	}
	return renderESLintSummary(summary), true
}

func parseESLintResults(raw string) ([]eslintResult, bool) {
	var results []eslintResult
	if json.Unmarshal([]byte(raw), &results) != nil {
		return nil, false
	}
	return results, true
}

func buildESLintSummaryData(results []eslintResult) eslintSummaryData {
	summary := eslintSummaryData{
		files: make([]eslintFileSummary, 0, len(results)),
	}
	ruleCounts := map[string]int{}
	for _, r := range results {
		summary.totalErrors += r.ErrorCount
		summary.totalWarnings += r.WarningCount
		fileSummary, hasIssues := buildESLintFileSummary(r)
		if !hasIssues {
			continue
		}
		summary.filesWithIssues++
		accumulateESLintRuleCounts(fileSummary.messages, ruleCounts)
		summary.files = append(summary.files, fileSummary)
	}
	sortESLintFiles(summary.files)
	summary.rules = buildSortedESLintRuleCounts(ruleCounts)
	return summary
}

func buildESLintFileSummary(r eslintResult) (eslintFileSummary, bool) {
	if len(r.Messages) == 0 {
		return eslintFileSummary{}, false
	}
	msgs := append([]eslintMessage{}, r.Messages...)
	sortESLintMessages(msgs)
	return eslintFileSummary{
		path:      r.FilePath,
		issues:    len(msgs),
		errorCnt:  r.ErrorCount,
		warnCnt:   r.WarningCount,
		messages:  msgs,
		shortPath: eslintShortPath(r.FilePath),
	}, true
}

func accumulateESLintRuleCounts(messages []eslintMessage, counts map[string]int) {
	for _, m := range messages {
		rule := strings.TrimSpace(m.RuleID)
		if rule == "" {
			continue
		}
		counts[rule]++
	}
}

func sortESLintMessages(msgs []eslintMessage) {
	sort.Slice(msgs, func(i, j int) bool {
		if msgs[i].Line != msgs[j].Line {
			return msgs[i].Line < msgs[j].Line
		}
		if msgs[i].Column != msgs[j].Column {
			return msgs[i].Column < msgs[j].Column
		}
		return msgs[i].RuleID < msgs[j].RuleID
	})
}

func eslintShortPath(filePath string) string {
	p := filepath.ToSlash(strings.TrimSpace(filePath))
	if p == "" {
		return filePath
	}
	const marker = "/src/"
	if idx := strings.Index(p, marker); idx >= 0 {
		return p[idx+1:]
	}
	return filepath.Base(p)
}

func sortESLintFiles(files []eslintFileSummary) {
	sort.Slice(files, func(i, j int) bool {
		if files[i].issues != files[j].issues {
			return files[i].issues > files[j].issues
		}
		return files[i].shortPath < files[j].shortPath
	})
}

func buildSortedESLintRuleCounts(ruleCounts map[string]int) []eslintRuleCount {
	rules := make([]eslintRuleCount, 0, len(ruleCounts))
	for rule, count := range ruleCounts {
		rules = append(rules, eslintRuleCount{rule: rule, count: count})
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].count != rules[j].count {
			return rules[i].count > rules[j].count
		}
		return rules[i].rule < rules[j].rule
	})
	return rules
}

func renderESLintSummary(summary eslintSummaryData) string {
	var b strings.Builder
	writeESLintHeader(&b, summary)
	writeTopESLintRules(&b, summary.rules)
	b.WriteString("top files:\n")
	for i, f := range summary.files {
		if i >= 5 {
			break
		}
		b.WriteString(fmt.Sprintf("- %s (%d issues)\n", f.shortPath, f.issues))
		writeTopESLintMessages(&b, f.messages)
	}
	return b.String()
}

func writeESLintHeader(b *strings.Builder, summary eslintSummaryData) {
	b.WriteString(fmt.Sprintf(
		"eslint: %d errors, %d warnings in %d files\n",
		summary.totalErrors,
		summary.totalWarnings,
		summary.filesWithIssues,
	))
}

func writeTopESLintRules(b *strings.Builder, rules []eslintRuleCount) {
	if len(rules) == 0 {
		return
	}
	b.WriteString("top rules:\n")
	for i, r := range rules {
		if i >= 5 {
			break
		}
		b.WriteString(fmt.Sprintf("- %s (%d)\n", r.rule, r.count))
	}
}

func writeTopESLintMessages(b *strings.Builder, messages []eslintMessage) {
	for j, m := range messages {
		if j >= 3 {
			break
		}
		b.WriteString(fmt.Sprintf(
			"  - %d:%d %s %s %s\n",
			m.Line,
			m.Column,
			eslintMessageSeverityLabel(m.Severity),
			eslintMessageRuleLabel(m.RuleID),
			strings.TrimSpace(m.Message),
		))
	}
}

func eslintMessageSeverityLabel(severity int) string {
	if severity == 2 {
		return "error"
	}
	return "warning"
}

func eslintMessageRuleLabel(ruleID string) string {
	rule := strings.TrimSpace(ruleID)
	if rule == "" {
		return "unknown-rule"
	}
	return rule
}
