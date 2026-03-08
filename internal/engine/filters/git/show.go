package gitfilters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
)

// NewGitShowFilter compacts default git show commit output with conservative passthrough.
func NewGitShowFilter() engine.ToolFilter { return gitShowFilter{} }

type gitShowFilter struct{}

type gitShowSummary struct {
	commitID string
	author   string
	date     string
	subject  string
	diffText string
}

const gitShowCommitPrefix = "commit "

func (gitShowFilter) Tool() string        { return "git show" }
func (gitShowFilter) Aliases() []string   { return nil }
func (gitShowFilter) MaskingHorizon() int { return 0 }
func (gitShowFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}

func (gitShowFilter) Prepare(args []string) engine.PrepareResult {
	out := make([]string, 0, len(args))
	forcePassthrough := false
	for _, arg := range args {
		if arg == "--no-compact" {
			forcePassthrough = true
			continue
		}
		if isGitShowPrecisionSensitiveArg(arg) || isGitShowBlobTarget(arg) {
			forcePassthrough = true
		}
		out = append(out, arg)
	}
	return engine.PrepareResult{NormalizedArgs: out, ForcePassthrough: forcePassthrough}
}

func (gitShowFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream == engine.StderrStream {
		if ev.Type == engine.EventLine {
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		}
		return engine.Decision{Action: engine.ActionIgnore}
	}
	if ev.Type != engine.EventEOF {
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := mem.Joined()
	if strings.TrimSpace(raw) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	if compacted, ok := compactGitShow(raw); ok {
		return engine.Decision{Action: engine.ActionFlush, Output: compacted}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: raw}
}

func isGitShowPrecisionSensitiveArg(arg string) bool {
	for _, prefix := range []string{
		"--format",
		"--pretty",
		"--raw",
		"--patch-with-raw",
		"--patch-with-stat",
		"--numstat",
		"--shortstat",
		"--stat",
		"--name-only",
		"--name-status",
		"--no-patch",
	} {
		if arg == prefix || strings.HasPrefix(arg, prefix+"=") {
			return true
		}
	}
	return arg == "-s"
}

func isGitShowBlobTarget(arg string) bool {
	if strings.HasPrefix(arg, "-") {
		return false
	}
	return strings.Contains(arg, ":")
}

func compactGitShow(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	summary, ok := parseGitShowSummary(strings.Split(trimmed, "\n"))
	if !ok {
		return "", false
	}

	var b strings.Builder
	b.WriteString(gitShowCommitPrefix)
	b.WriteString(shortGitShowCommitID(summary.commitID))
	b.WriteString("\n")
	if summary.author != "" {
		b.WriteString("author: ")
		b.WriteString(summary.author)
		b.WriteString("\n")
	}
	if summary.date != "" {
		b.WriteString("date: ")
		b.WriteString(summary.date)
		b.WriteString("\n")
	}
	if summary.subject != "" {
		b.WriteString("subject: ")
		b.WriteString(summary.subject)
		b.WriteString("\n")
	}

	if summary.diffText != "" {
		diffSummary := compactDiff(summary.diffText)
		if !strings.Contains(diffSummary, "summary: ") {
			return "", false
		}
		b.WriteString(diffSummary)
	}
	return b.String(), true
}

func parseGitShowSummary(lines []string) (gitShowSummary, bool) {
	if len(lines) == 0 || !strings.HasPrefix(lines[0], gitShowCommitPrefix) {
		return gitShowSummary{}, false
	}

	summary := gitShowSummary{
		commitID: strings.TrimSpace(strings.TrimPrefix(lines[0], gitShowCommitPrefix)),
	}
	if summary.commitID == "" {
		return gitShowSummary{}, false
	}

	diffStart, ok := populateGitShowSummary(&summary, lines[1:])
	if !ok {
		return gitShowSummary{}, false
	}
	if diffStart >= 0 {
		summary.diffText = strings.Join(lines[diffStart:], "\n")
	}
	return summary, true
}

func populateGitShowSummary(summary *gitShowSummary, lines []string) (int, bool) {
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "Author:"):
			summary.author = strings.TrimSpace(strings.TrimPrefix(line, "Author:"))
		case strings.HasPrefix(line, "Date:"):
			summary.date = strings.TrimSpace(strings.TrimPrefix(line, "Date:"))
		case strings.HasPrefix(line, "diff --git "):
			return i + 1, true
		case summary.subject == "" && strings.HasPrefix(line, "    ") && strings.TrimSpace(line) != "":
			summary.subject = strings.TrimSpace(line)
		case strings.TrimSpace(line) == "":
			continue
		case strings.HasPrefix(line, "    "):
			continue
		default:
			return -1, false
		}
	}
	return -1, true
}

func shortGitShowCommitID(commitID string) string {
	if len(commitID) > 12 {
		return commitID[:12]
	}
	return commitID
}
