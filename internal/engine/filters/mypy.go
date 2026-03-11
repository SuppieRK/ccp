package filters

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go-command-compression-proxy/internal/engine"
)

var (
	mypyDiagRe = regexp.MustCompile(`^(.+?):(\d+)(?::\d+)?:\s+(error|warning|note):\s+(.+?)(?:\s+\[(.+)\])?$`)
)

const (
	mypyDispatchKey        = "mypy"
	mypyMaxFiles           = 5
	mypyMaxIssuesPerFile   = 4
	mypyErrorPrefix        = "error:"
	mypySuccessPrefix      = "Success:"
	mypyFoundSummaryPrefix = "Found "
)

func NewMypyFilter() engine.ToolFilter { return mypyFilter{} }

type mypyFilter struct{}

func (mypyFilter) Tool() string { return "mypy" }

func (mypyFilter) Aliases() []string {
	return []string{"mypy.exe", "./mypy", "./mypy.exe", "mypy.cmd", "./mypy.cmd"}
}

func (mypyFilter) MaskingHorizon() int { return 0 }

func (mypyFilter) Prepare(args []string) engine.PrepareResult {
	prep := engine.PrepareResult{
		NormalizedArgs:   append([]string{}, args...),
		ForcePassthrough: true,
	}
	if !supportsMypyCompaction(args) {
		return prep
	}
	prep.ForcePassthrough = false
	prep.DispatchKey = mypyDispatchKey
	return prep
}

func supportsMypyCompaction(args []string) bool {
	if len(nonEmptyArgs(args)) == 0 {
		return true
	}
	return !mypyWantsStructuredPassthrough(args)
}

func mypyWantsStructuredPassthrough(args []string) bool {
	for i := 0; i < len(args); i++ {
		v := strings.ToLower(strings.TrimSpace(args[i]))
		if v == "" {
			continue
		}
		if mypyStructuredFlag(v) {
			return true
		}
		if mypyStructuredFlagWithValue(v, args, i) {
			return true
		}
	}
	return false
}

func mypyStructuredFlag(v string) bool {
	switch {
	case strings.HasPrefix(v, "--junit-xml"),
		strings.HasPrefix(v, "--html-report"),
		strings.HasPrefix(v, "--txt-report"),
		strings.HasPrefix(v, "--xml-report"),
		strings.HasPrefix(v, "--xslt-html-report"),
		strings.HasPrefix(v, "--xslt-txt-report"),
		strings.HasPrefix(v, "--linecount-report"),
		strings.HasPrefix(v, "--linecoverage-report"),
		strings.HasPrefix(v, "--cobertura-xml-report"):
		return true
	default:
		return false
	}
}

func mypyStructuredFlagWithValue(v string, args []string, i int) bool {
	if strings.HasPrefix(v, "--output=") || strings.HasPrefix(v, "--error-format=") {
		return true
	}
	if i+1 >= len(args) {
		return false
	}
	if v == "--output" || v == "--error-format" {
		return true
	}
	return false
}

func (mypyFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}

func (mypyFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
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
		out, ok := compactMypyOutput(raw)
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

type mypyIssue struct {
	file    string
	line    string
	code    string
	message string
	notes   []string
}

type mypyFileSummary struct {
	path   string
	short  string
	issues []mypyIssue
}

type mypyCodeSummary struct {
	code  string
	count int
}

func compactMypyOutput(raw string) (string, bool) {
	if strings.ContainsRune(raw, '\x00') {
		return "", false
	}
	lines := nonEmptyLines(raw)
	if len(lines) == 0 {
		return "", false
	}
	parse := parseMypy(lines)
	if !parse.recognized {
		return "", false
	}
	return renderMypy(parse), true
}

func nonEmptyLines(raw string) []string {
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

type mypyParse struct {
	recognized bool
	success    bool
	fileless   []string
	files      []mypyFileSummary
}

func parseMypy(lines []string) mypyParse {
	issues := make([]mypyIssue, 0, 16)
	fileless := make([]string, 0, 4)
	recognized := false
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(trimmed, mypySuccessPrefix):
			return mypyParse{recognized: true, success: true}
		case strings.HasPrefix(trimmed, mypyFoundSummaryPrefix) && strings.Contains(trimmed, " error"):
			recognized = true
			continue
		}

		issue, ok := parseMypyIssue(trimmed)
		if !ok {
			if strings.Contains(trimmed, mypyErrorPrefix) {
				fileless = append(fileless, trimmed)
				recognized = true
			}
			continue
		}
		recognized = true
		for i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			note, noteOK := parseMypyNote(next, issue.file)
			if !noteOK {
				break
			}
			issue.notes = append(issue.notes, note)
			i++
		}
		issues = append(issues, issue)
	}

	if !recognized {
		return mypyParse{}
	}
	files, _ := buildMypySummaries(issues)
	return mypyParse{recognized: true, fileless: fileless, files: files}
}

func parseMypyIssue(line string) (mypyIssue, bool) {
	m := mypyDiagRe.FindStringSubmatch(line)
	if len(m) != 6 {
		return mypyIssue{}, false
	}
	severity := strings.TrimSpace(m[3])
	if severity != "error" && severity != "warning" {
		return mypyIssue{}, false
	}
	file := strings.TrimSpace(m[1])
	lineNo := strings.TrimSpace(m[2])
	msg := strings.TrimSpace(m[4])
	code := strings.TrimSpace(m[5])
	return mypyIssue{file: file, line: lineNo, code: code, message: msg}, file != "" && lineNo != ""
}

func parseMypyNote(line, file string) (string, bool) {
	m := mypyDiagRe.FindStringSubmatch(line)
	if len(m) != 6 {
		return "", false
	}
	if strings.TrimSpace(m[1]) != file || strings.TrimSpace(m[3]) != "note" {
		return "", false
	}
	return strings.TrimSpace(m[4]), true
}

func buildMypySummaries(issues []mypyIssue) ([]mypyFileSummary, []mypyCodeSummary) {
	fileMap := map[string][]mypyIssue{}
	codeMap := map[string]int{}
	for _, issue := range issues {
		fileMap[issue.file] = append(fileMap[issue.file], issue)
		if issue.code != "" {
			codeMap[issue.code]++
		}
	}
	files := make([]mypyFileSummary, 0, len(fileMap))
	for path, fileIssues := range fileMap {
		sortMypyIssues(fileIssues)
		files = append(files, mypyFileSummary{path: path, short: mypyShortPath(path), issues: fileIssues})
	}
	sort.Slice(files, func(i, j int) bool {
		if len(files[i].issues) != len(files[j].issues) {
			return len(files[i].issues) > len(files[j].issues)
		}
		return files[i].short < files[j].short
	})
	codes := make([]mypyCodeSummary, 0, len(codeMap))
	for code, count := range codeMap {
		codes = append(codes, mypyCodeSummary{code: code, count: count})
	}
	sort.Slice(codes, func(i, j int) bool {
		if codes[i].count != codes[j].count {
			return codes[i].count > codes[j].count
		}
		return codes[i].code < codes[j].code
	})
	return files, codes
}

func sortMypyIssues(issues []mypyIssue) {
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].line != issues[j].line {
			return issues[i].line < issues[j].line
		}
		if issues[i].code != issues[j].code {
			return issues[i].code < issues[j].code
		}
		return issues[i].message < issues[j].message
	})
}

func mypyShortPath(path string) string {
	p := filepath.ToSlash(strings.TrimSpace(path))
	if p == "" {
		return path
	}
	for _, marker := range []string{"src/", "pkg/", "cmd/", "internal/"} {
		if strings.HasPrefix(p, marker) {
			return p
		}
	}
	for _, marker := range []string{"/src/", "/pkg/", "/cmd/", "/internal/"} {
		if idx := strings.Index(p, marker); idx >= 0 {
			return p[idx+1:]
		}
	}
	return filepath.Base(p)
}

func renderMypy(parse mypyParse) string {
	if parse.success {
		return "mypy: No issues found\n"
	}
	var b strings.Builder
	writeMypyFileless(&b, parse.fileless)
	writeMypyHeader(&b, parse.files)
	writeMypyFiles(&b, parse.files)
	return b.String()
}

func writeMypyFileless(b *strings.Builder, lines []string) {
	for _, line := range lines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	if len(lines) > 0 {
		b.WriteString("\n")
	}
}

func writeMypyHeader(b *strings.Builder, files []mypyFileSummary) {
	b.WriteString(fmt.Sprintf("mypy: %d errors in %d files\n", totalMypyIssues(files), len(files)))
}

func totalMypyIssues(files []mypyFileSummary) int {
	total := 0
	for _, file := range files {
		total += len(file.issues)
	}
	return total
}

func writeMypyFiles(b *strings.Builder, files []mypyFileSummary) {
	limit := min(len(files), mypyMaxFiles)
	for _, file := range files[:limit] {
		b.WriteString(fmt.Sprintf("- %s (%d errors)\n", file.short, len(file.issues)))
		for _, line := range mypyIssueLines(file.issues) {
			b.WriteString(line)
		}
	}
	if len(files) > mypyMaxFiles {
		b.WriteString(fmt.Sprintf("+ %d more files\n", len(files)-mypyMaxFiles))
	}
}

func mypyIssueLines(issues []mypyIssue) []string {
	lines := make([]string, 0, min(len(issues), mypyMaxIssuesPerFile)+1)
	for i, issue := range issues {
		if i >= mypyMaxIssuesPerFile {
			lines = append(lines, fmt.Sprintf("  + %d more issues\n", len(issues)-i))
			break
		}
		lines = append(lines, "  "+renderMypyIssue(issue)+"\n")
		for _, note := range issue.notes {
			lines = append(lines, "    "+note+"\n")
		}
	}
	return lines
}

func renderMypyIssue(issue mypyIssue) string {
	if issue.code == "" {
		return fmt.Sprintf("L%s %s", issue.line, issue.message)
	}
	return fmt.Sprintf("L%s [%s] %s", issue.line, issue.code, issue.message)
}
