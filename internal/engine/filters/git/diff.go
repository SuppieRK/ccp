package gitfilters

import (
	"fmt"
	"strings"

	"go-command-compression-proxy/internal/engine"
)

// NewGitDiffFilter compacts git diff output with passthrough escape hatches.
func NewGitDiffFilter() engine.ToolFilter { return gitDiffFilter{} }

type gitDiffFilter struct{}

type fileStat struct {
	name    string
	added   int
	removed int
	snippet []string
}

func (gitDiffFilter) Tool() string        { return "git diff" }
func (gitDiffFilter) Aliases() []string   { return nil }
func (gitDiffFilter) MaskingHorizon() int { return 0 }
func (gitDiffFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}
func (gitDiffFilter) Prepare(args []string) engine.PrepareResult {
	out := make([]string, 0, len(args))
	forcePassthrough := false
	for _, arg := range args {
		if arg == "--no-compact" {
			forcePassthrough = true
			continue
		}
		if arg == "--stat" || arg == "--numstat" || arg == "--shortstat" {
			forcePassthrough = true
		}
		out = append(out, arg)
	}
	return engine.PrepareResult{NormalizedArgs: out, ForcePassthrough: forcePassthrough}
}

func (gitDiffFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream == engine.StderrStream {
		if ev.Type == engine.EventLine {
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		}
		return engine.Decision{Action: engine.ActionIgnore}
	}
	if ev.Type != engine.EventEOF {
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := strings.TrimSpace(mem.Joined())
	if raw == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: compactDiff(raw)}
}

func compactDiff(raw string) string {
	const maxSnippetLines = 6
	lines := strings.Split(raw, "\n")
	files := make([]fileStat, 0)
	cur := fileStat{}
	inHunk := false
	hunkLines := 0

	flush := func() {
		if cur.name == "" {
			return
		}
		files = append(files, cur)
		cur = fileStat{}
		inHunk = false
		hunkLines = 0
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			startDiffFile(&cur, line)
			inHunk = false
			hunkLines = 0
			continue
		}
		if strings.HasPrefix(line, "@@") {
			inHunk = true
			hunkLines = 0
			continue
		}
		if !inHunk {
			continue
		}
		if recordDiffChangeLine(&cur, line, &hunkLines, maxSnippetLines) {
			continue
		}
	}
	flush()

	if len(files) == 0 {
		return raw + "\n"
	}

	var b strings.Builder
	totalAdded := 0
	totalRemoved := 0
	for _, f := range files {
		totalAdded += f.added
		totalRemoved += f.removed
		_, _ = fmt.Fprintf(&b, "%s  +%d -%d\n", f.name, f.added, f.removed)
		for _, line := range f.snippet {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	_, _ = fmt.Fprintf(&b, "summary: %d files changed, +%d -%d\n", len(files), totalAdded, totalRemoved)
	return b.String()
}

func startDiffFile(cur *fileStat, line string) {
	if _, rhs, ok := strings.Cut(line, " b/"); ok {
		cur.name = rhs
		return
	}
	cur.name = "unknown"
}

func recordDiffChangeLine(cur *fileStat, line string, hunkLines *int, maxSnippetLines int) bool {
	if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
		cur.added++
		appendDiffSnippetLine(cur, line, hunkLines, maxSnippetLines)
		return true
	}
	if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
		cur.removed++
		appendDiffSnippetLine(cur, line, hunkLines, maxSnippetLines)
		return true
	}
	return false
}

func appendDiffSnippetLine(cur *fileStat, line string, hunkLines *int, maxSnippetLines int) {
	if *hunkLines >= maxSnippetLines {
		return
	}
	cur.snippet = append(cur.snippet, line)
	*hunkLines = *hunkLines + 1
}
