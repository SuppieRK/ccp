package filters

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"go-command-compression-proxy/internal/engine"
)

const (
	ruffDispatchKey      = "ruff"
	ruffStructuredReason = "structured output mode"
	ruffOutputFormatFlag = "--output-format"
	ruffMaxFiles         = 5
	ruffMaxIssuesPerFile = 4
	ruffDefaultCheckPath = "."
)

func NewRuffFilter() engine.ToolFilter { return ruffFilter{} }

type ruffFilter struct{}

func (ruffFilter) Tool() string { return "ruff" }

func (ruffFilter) Aliases() []string {
	return []string{"ruff.exe", "./ruff", "./ruff.exe", "ruff.cmd", "./ruff.cmd"}
}

func (ruffFilter) MaskingHorizon() int { return 0 }

func (ruffFilter) Prepare(args []string) engine.PrepareResult {
	prep := engine.PrepareResult{NormalizedArgs: append([]string{}, args...), ForcePassthrough: true}
	normalized, ok, ambiguous := normalizeRuffArgs(args)
	if !ok {
		if ambiguous {
			prep.Ambiguous = true
			prep.Reason = ruffStructuredReason
		}
		return prep
	}
	prep.NormalizedArgs = normalized
	prep.ForcePassthrough = false
	prep.DispatchKey = ruffDispatchKey
	return prep
}

func normalizeRuffArgs(args []string) ([]string, bool, bool) {
	trimmed := nonEmptyArgs(args)
	if len(trimmed) == 0 {
		return ruffDefaultCheckArgs(nil), true, false
	}
	if ruffWantsStructuredPassthrough(trimmed) {
		return nil, false, true
	}
	if ruffWantsPassthrough(trimmed) {
		return nil, false, false
	}
	if strings.EqualFold(trimmed[0], "check") {
		return ruffDefaultCheckArgs(trimmed[1:]), true, false
	}
	if strings.HasPrefix(strings.TrimSpace(trimmed[0]), "-") || ruffLooksLikePathArg(trimmed[0]) {
		return ruffDefaultCheckArgs(trimmed), true, false
	}
	return nil, false, false
}

func ruffDefaultCheckArgs(args []string) []string {
	out := []string{"check", ruffOutputFormatFlag, "json"}
	out = append(out, args...)
	if ruffNeedsDefaultPath(args) {
		out = append(out, ruffDefaultCheckPath)
	}
	return out
}

func ruffNeedsDefaultPath(args []string) bool {
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "-") {
			return false
		}
	}
	return true
}

func ruffLooksLikePathArg(arg string) bool {
	trimmed := strings.TrimSpace(arg)
	if trimmed == "" || strings.HasPrefix(trimmed, "-") {
		return false
	}
	return true
}

func ruffWantsStructuredPassthrough(args []string) bool {
	for i := 0; i < len(args); i++ {
		v := strings.ToLower(strings.TrimSpace(args[i]))
		if v == "" {
			continue
		}
		if strings.HasPrefix(v, ruffOutputFormatFlag+"=") || v == ruffOutputFormatFlag {
			return true
		}
		if strings.HasPrefix(v, "--output-file") || strings.HasPrefix(v, "--statistics") {
			return true
		}
	}
	return false
}

func ruffWantsPassthrough(args []string) bool {
	for _, arg := range args {
		v := strings.ToLower(strings.TrimSpace(arg))
		switch v {
		case "format", "server", "analyze", "version", "--version", "help", "-h", "--help", "--fix", "clean":
			return true
		}
	}
	return false
}

func (ruffFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}

func (ruffFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream == engine.StderrStream {
		switch ev.Type {
		case engine.EventLine:
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		default:
			return engine.Decision{Action: engine.ActionIgnore}
		}
	}
	switch ev.Type {
	case engine.EventLine, engine.EventTick:
		return engine.Decision{Action: engine.ActionCollect}
	case engine.EventEOF:
		return engine.Decision{Action: engine.ActionIgnore}
	case engine.EventExit:
		raw := mem.Joined()
		if strings.TrimSpace(raw) == "" {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		out, ok := summarizeRuffJSON(raw)
		if !ok {
			return engine.Decision{Action: engine.ActionFlush, Output: raw}
		}
		if strings.TrimSpace(out) == "" {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: out}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

type ruffLocation struct {
	Row    int `json:"row"`
	Column int `json:"column"`
}

type ruffFix struct {
	Applicability string `json:"applicability"`
}

type ruffDiagnostic struct {
	Code     string       `json:"code"`
	Message  string       `json:"message"`
	Location ruffLocation `json:"location"`
	Filename string       `json:"filename"`
	Fix      *ruffFix     `json:"fix"`
}

type ruffRenderedIssue struct {
	line    int
	column  int
	code    string
	message string
	fixable bool
}

type ruffFileSummary struct {
	path   string
	short  string
	issues []ruffRenderedIssue
}

type ruffRuleSummary struct {
	code  string
	count int
}

func summarizeRuffJSON(raw string) (string, bool) {
	var parsed []ruffDiagnostic
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return "", false
	}
	if len(parsed) == 0 {
		return "", true
	}
	files, rules, fixable := buildRuffSummary(parsed)
	return renderRuffSummary(files, rules, fixable), true
}

func buildRuffSummary(diags []ruffDiagnostic) ([]ruffFileSummary, []ruffRuleSummary, int) {
	fileMap := map[string][]ruffRenderedIssue{}
	ruleMap := map[string]int{}
	fixable := 0
	for _, diag := range diags {
		path, rendered := buildRuffRenderedIssue(diag)
		if rendered.fixable {
			fixable++
		}
		fileMap[path] = append(fileMap[path], rendered)
		if rendered.code != "" {
			ruleMap[rendered.code]++
		}
	}
	return buildRuffFileSummaries(fileMap), buildRuffRuleSummaries(ruleMap), fixable
}

func buildRuffRenderedIssue(diag ruffDiagnostic) (string, ruffRenderedIssue) {
	path := ruffPath(diag.Filename)
	return path, ruffRenderedIssue{
		line:    diag.Location.Row,
		column:  diag.Location.Column,
		code:    strings.TrimSpace(diag.Code),
		message: strings.TrimSpace(diag.Message),
		fixable: diag.Fix != nil,
	}
}

func buildRuffFileSummaries(fileMap map[string][]ruffRenderedIssue) []ruffFileSummary {
	files := make([]ruffFileSummary, 0, len(fileMap))
	for path, issues := range fileMap {
		sortRuffIssues(issues)
		files = append(files, ruffFileSummary{path: path, short: ruffShortPath(path), issues: issues})
	}
	sort.Slice(files, func(i, j int) bool {
		if len(files[i].issues) != len(files[j].issues) {
			return len(files[i].issues) > len(files[j].issues)
		}
		return files[i].short < files[j].short
	})
	return files
}

func sortRuffIssues(issues []ruffRenderedIssue) {
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].line != issues[j].line {
			return issues[i].line < issues[j].line
		}
		if issues[i].column != issues[j].column {
			return issues[i].column < issues[j].column
		}
		if issues[i].code != issues[j].code {
			return issues[i].code < issues[j].code
		}
		return issues[i].message < issues[j].message
	})
}

func buildRuffRuleSummaries(ruleMap map[string]int) []ruffRuleSummary {
	rules := make([]ruffRuleSummary, 0, len(ruleMap))
	for code, count := range ruleMap {
		rules = append(rules, ruffRuleSummary{code: code, count: count})
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].count != rules[j].count {
			return rules[i].count > rules[j].count
		}
		return rules[i].code < rules[j].code
	})
	return rules
}

func ruffPath(path string) string {
	p := filepath.ToSlash(strings.TrimSpace(path))
	if p == "" {
		return "<unknown>"
	}
	return p
}

func ruffShortPath(path string) string {
	p := filepath.ToSlash(strings.TrimSpace(path))
	if p == "" {
		return path
	}
	for _, marker := range []string{"src/", "tests/", "pkg/", "cmd/", "internal/"} {
		if strings.HasPrefix(p, marker) {
			return p
		}
	}
	for _, marker := range []string{"/src/", "/tests/", "/pkg/", "/cmd/", "/internal/"} {
		if idx := strings.Index(p, marker); idx >= 0 {
			return p[idx+1:]
		}
	}
	return filepath.Base(p)
}

func renderRuffSummary(files []ruffFileSummary, rules []ruffRuleSummary, fixable int) string {
	var b strings.Builder
	writeRuffHeader(&b, files, fixable)
	writeRuffRules(&b, rules)
	writeRuffFiles(&b, files)
	return b.String()
}

func writeRuffHeader(b *strings.Builder, files []ruffFileSummary, fixable int) {
	_, _ = fmt.Fprintf(b, "ruff: %d issues in %d files", totalRuffIssues(files), len(files))
	if fixable > 0 {
		_, _ = fmt.Fprintf(b, " (%d fixable)", fixable)
	}
	b.WriteString("\n")
}

func totalRuffIssues(files []ruffFileSummary) int {
	total := 0
	for _, file := range files {
		total += len(file.issues)
	}
	return total
}

func writeRuffRules(b *strings.Builder, rules []ruffRuleSummary) {
	if len(rules) == 0 {
		return
	}
	b.WriteString("top rules:\n")
	for _, rule := range rules[:min(len(rules), ruffMaxFiles)] {
		_, _ = fmt.Fprintf(b, "- %s (%d)\n", rule.code, rule.count)
	}
}

func writeRuffFiles(b *strings.Builder, files []ruffFileSummary) {
	for _, file := range files[:min(len(files), ruffMaxFiles)] {
		_, _ = fmt.Fprintf(b, "- %s (%d issues)\n", file.short, len(file.issues))
		writeRuffIssues(b, file.issues)
	}
	if len(files) > ruffMaxFiles {
		_, _ = fmt.Fprintf(b, "+ %d more files\n", len(files)-ruffMaxFiles)
	}
}

func writeRuffIssues(b *strings.Builder, issues []ruffRenderedIssue) {
	for i, issue := range issues {
		if i >= ruffMaxIssuesPerFile {
			_, _ = fmt.Fprintf(b, "  + %d more issues\n", len(issues)-i)
			break
		}
		writeRuffIssue(b, issue)
	}
}

func writeRuffIssue(b *strings.Builder, issue ruffRenderedIssue) {
	line := ruffIssueLocation(issue)
	if issue.code != "" {
		_, _ = fmt.Fprintf(b, "  %s %s %s\n", line, issue.code, issue.message)
		return
	}
	_, _ = fmt.Fprintf(b, "  %s %s\n", line, issue.message)
}

func ruffIssueLocation(issue ruffRenderedIssue) string {
	if issue.line == 0 && issue.column == 0 {
		return "?:?"
	}
	return fmt.Sprintf("%d:%d", issue.line, issue.column)
}
