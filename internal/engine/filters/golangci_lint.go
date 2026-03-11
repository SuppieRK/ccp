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
	golangciLintDispatchKey      = "golangci-lint"
	golangciLintOutFormatFlag    = "--out-format"
	golangciLintMaxFiles         = 5
	golangciLintMaxIssuesPerFile = 3
)

func NewGolangciLintFilter() engine.ToolFilter { return golangciLintFilter{} }

type golangciLintFilter struct{}

func (golangciLintFilter) Tool() string { return "golangci-lint" }

func (golangciLintFilter) Aliases() []string {
	return []string{
		"golangci-lint.exe",
		"./golangci-lint",
		"./golangci-lint.exe",
		"golangci-lint.cmd",
		"./golangci-lint.cmd",
	}
}

func (golangciLintFilter) MaskingHorizon() int { return 0 }

func (golangciLintFilter) Prepare(args []string) engine.PrepareResult {
	prep := engine.PrepareResult{
		NormalizedArgs:   append([]string{}, args...),
		ForcePassthrough: true,
	}
	normalized, ok := normalizeGolangciLintArgs(args)
	if !ok {
		return prep
	}
	prep.NormalizedArgs = normalized
	prep.ForcePassthrough = false
	prep.DispatchKey = golangciLintDispatchKey
	return prep
}

func normalizeGolangciLintArgs(args []string) ([]string, bool) {
	if len(args) == 0 {
		return golangciLintDefaultRunArgs(nil), true
	}
	trimmed := nonEmptyArgs(args)
	if len(trimmed) == 0 {
		return golangciLintDefaultRunArgs(nil), true
	}
	if golangciLintWantsStructuredPassthrough(trimmed) || golangciLintWantsPassthrough(trimmed) {
		return nil, false
	}
	if golangciLintHasUnsupportedSubcommand(trimmed) {
		return nil, false
	}
	if strings.EqualFold(trimmed[0], "run") {
		return golangciLintDefaultRunArgs(trimmed[1:]), true
	}
	return golangciLintDefaultRunArgs(trimmed), true
}

func golangciLintDefaultRunArgs(args []string) []string {
	base := []string{"run", golangciLintOutFormatFlag, "json"}
	return append(base, args...)
}

func nonEmptyArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.TrimSpace(arg) == "" {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func golangciLintWantsStructuredPassthrough(args []string) bool {
	for _, arg := range args {
		v := strings.ToLower(strings.TrimSpace(arg))
		if v == golangciLintOutFormatFlag || strings.HasPrefix(v, golangciLintOutFormatFlag+"=") {
			return true
		}
		if strings.HasPrefix(v, "--output.") {
			return true
		}
	}
	return false
}

func golangciLintWantsPassthrough(args []string) bool {
	for _, arg := range args {
		v := strings.ToLower(strings.TrimSpace(arg))
		switch v {
		case "-h", "--help", "help", "version", "--version", "--fix":
			return true
		}
	}
	return false
}

func golangciLintHasUnsupportedSubcommand(args []string) bool {
	first := strings.ToLower(strings.TrimSpace(args[0]))
	if strings.HasPrefix(first, "-") || first == "" || first == "run" {
		return false
	}
	switch first {
	case "cache", "completion", "config", "custom", "formatters", "linters", "migrate", "version", "help":
		return true
	default:
		return false
	}
}

func (golangciLintFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}

func (golangciLintFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
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
		out, ok := summarizeGolangciLintJSON(raw)
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

type golangciLintIssuePosition struct {
	Filename string `json:"Filename"`
	Line     int    `json:"Line"`
	Column   int    `json:"Column"`
}

type golangciLintIssue struct {
	FromLinter string                    `json:"FromLinter"`
	Text       string                    `json:"Text"`
	Pos        golangciLintIssuePosition `json:"Pos"`
}

type golangciLintJSON struct {
	Issues []golangciLintIssue `json:"Issues"`
}

type golangciLintRenderedIssue struct {
	line   int
	column int
	linter string
	text   string
}

type golangciLintFileSummary struct {
	path   string
	short  string
	issues []golangciLintRenderedIssue
}

type golangciLintRuleSummary struct {
	name  string
	count int
}

func summarizeGolangciLintJSON(raw string) (string, bool) {
	var parsed golangciLintJSON
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return "", false
	}
	if len(parsed.Issues) == 0 {
		return "", true
	}
	files, rules := buildGolangciLintSummary(parsed.Issues)
	return renderGolangciLintSummary(files, rules), true
}

func buildGolangciLintSummary(issues []golangciLintIssue) ([]golangciLintFileSummary, []golangciLintRuleSummary) {
	fileMap := map[string][]golangciLintRenderedIssue{}
	ruleMap := map[string]int{}
	for _, issue := range issues {
		path := golangciLintIssuePath(issue)
		rendered := newGolangciLintRenderedIssue(issue)
		fileMap[path] = append(fileMap[path], rendered)
		if rendered.linter != "" {
			ruleMap[rendered.linter]++
		}
	}
	return buildGolangciLintFileSummaries(fileMap), buildGolangciLintRuleSummaries(ruleMap)
}

func golangciLintIssuePath(issue golangciLintIssue) string {
	path := filepath.ToSlash(strings.TrimSpace(issue.Pos.Filename))
	if path == "" {
		return "<unknown>"
	}
	return path
}

func newGolangciLintRenderedIssue(issue golangciLintIssue) golangciLintRenderedIssue {
	return golangciLintRenderedIssue{
		line:   issue.Pos.Line,
		column: issue.Pos.Column,
		linter: strings.TrimSpace(issue.FromLinter),
		text:   strings.TrimSpace(issue.Text),
	}
}

func buildGolangciLintFileSummaries(fileMap map[string][]golangciLintRenderedIssue) []golangciLintFileSummary {
	files := make([]golangciLintFileSummary, 0, len(fileMap))
	for path, fileIssues := range fileMap {
		sortGolangciLintIssues(fileIssues)
		files = append(files, golangciLintFileSummary{
			path:   path,
			short:  golangciLintShortPath(path),
			issues: fileIssues,
		})
	}
	sortGolangciLintFiles(files)
	return files
}

func sortGolangciLintIssues(issues []golangciLintRenderedIssue) {
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].line != issues[j].line {
			return issues[i].line < issues[j].line
		}
		if issues[i].column != issues[j].column {
			return issues[i].column < issues[j].column
		}
		if issues[i].linter != issues[j].linter {
			return issues[i].linter < issues[j].linter
		}
		return issues[i].text < issues[j].text
	})
}

func sortGolangciLintFiles(files []golangciLintFileSummary) {
	sort.Slice(files, func(i, j int) bool {
		if len(files[i].issues) != len(files[j].issues) {
			return len(files[i].issues) > len(files[j].issues)
		}
		return files[i].short < files[j].short
	})
}

func buildGolangciLintRuleSummaries(ruleMap map[string]int) []golangciLintRuleSummary {
	rules := make([]golangciLintRuleSummary, 0, len(ruleMap))
	for name, count := range ruleMap {
		rules = append(rules, golangciLintRuleSummary{name: name, count: count})
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].count != rules[j].count {
			return rules[i].count > rules[j].count
		}
		return rules[i].name < rules[j].name
	})
	return rules
}

func golangciLintShortPath(path string) string {
	p := filepath.ToSlash(strings.TrimSpace(path))
	if p == "" {
		return path
	}
	for _, marker := range []string{"pkg/", "cmd/", "internal/"} {
		if strings.HasPrefix(p, marker) {
			return p
		}
	}
	for _, marker := range []string{"/pkg/", "/cmd/", "/internal/"} {
		if idx := strings.Index(p, marker); idx >= 0 {
			return p[idx+1:]
		}
	}
	return filepath.Base(p)
}

func renderGolangciLintSummary(files []golangciLintFileSummary, rules []golangciLintRuleSummary) string {
	var b strings.Builder
	writeGolangciLintHeader(&b, files)
	writeGolangciLintRules(&b, rules)
	writeGolangciLintFiles(&b, files)
	return b.String()
}

func writeGolangciLintHeader(b *strings.Builder, files []golangciLintFileSummary) {
	_, _ = fmt.Fprintf(b, "golangci-lint: %d issues in %d files\n", golangciLintTotalIssues(files), len(files))
}

func golangciLintTotalIssues(files []golangciLintFileSummary) int {
	total := 0
	for _, file := range files {
		total += len(file.issues)
	}
	return total
}

func writeGolangciLintRules(b *strings.Builder, rules []golangciLintRuleSummary) {
	if len(rules) == 0 {
		return
	}
	b.WriteString("top linters:\n")
	for _, rule := range limitedGolangciLintRules(rules) {
		_, _ = fmt.Fprintf(b, "- %s (%d)\n", rule.name, rule.count)
	}
}

func limitedGolangciLintRules(rules []golangciLintRuleSummary) []golangciLintRuleSummary {
	if len(rules) <= golangciLintMaxFiles {
		return rules
	}
	return rules[:golangciLintMaxFiles]
}

func writeGolangciLintFiles(b *strings.Builder, files []golangciLintFileSummary) {
	b.WriteString("files:\n")
	for _, file := range limitedGolangciLintFiles(files) {
		writeGolangciLintFileSummary(b, file)
	}
	if len(files) > golangciLintMaxFiles {
		_, _ = fmt.Fprintf(b, "+ %d more files\n", len(files)-golangciLintMaxFiles)
	}
}

func limitedGolangciLintFiles(files []golangciLintFileSummary) []golangciLintFileSummary {
	if len(files) <= golangciLintMaxFiles {
		return files
	}
	return files[:golangciLintMaxFiles]
}

func writeGolangciLintFileSummary(b *strings.Builder, file golangciLintFileSummary) {
	_, _ = fmt.Fprintf(b, "- %s (%d issues)\n", file.short, len(file.issues))
	for _, line := range golangciLintIssueLines(file.issues) {
		b.WriteString(line)
	}
}

func golangciLintIssueLines(issues []golangciLintRenderedIssue) []string {
	lines := make([]string, 0, min(len(issues), golangciLintMaxIssuesPerFile)+1)
	for i, issue := range issues {
		if i >= golangciLintMaxIssuesPerFile {
			lines = append(lines, fmt.Sprintf("  + %d more issues\n", len(issues)-i))
			break
		}
		lines = append(lines, "  "+renderGolangciLintIssue(issue)+"\n")
	}
	return lines
}

func renderGolangciLintIssue(issue golangciLintRenderedIssue) string {
	location := fmt.Sprintf("%d:%d", issue.line, issue.column)
	if issue.line == 0 && issue.column == 0 {
		location = "?:?"
	}
	if issue.linter == "" {
		return location + " " + issue.text
	}
	return location + " " + issue.linter + " " + issue.text
}
