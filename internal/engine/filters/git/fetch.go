package gitfilters

import (
	"fmt"
	"strings"

	"go-command-compression-proxy/internal/engine"
)

// NewGitFetchFilter compacts non-empty successful git fetch output.
func NewGitFetchFilter() engine.ToolFilter { return gitFetchFilter{} }

type gitFetchFilter struct{}

type gitFetchSummary struct {
	refUpdates  int
	newBranches int
	newTags     int
	newRefs     int
	forced      int
}

func (gitFetchFilter) Tool() string        { return "git fetch" }
func (gitFetchFilter) Aliases() []string   { return nil }
func (gitFetchFilter) MaskingHorizon() int { return 0 }
func (gitFetchFilter) ContextKey(ev engine.Event) string {
	return sharedContextKey(ev)
}

func (gitFetchFilter) Prepare(args []string) engine.PrepareResult {
	out := make([]string, 0, len(args))
	forcePassthrough := false
	for _, arg := range args {
		if arg == "--no-compact" {
			forcePassthrough = true
			continue
		}
		if isGitFetchDetailArg(arg) {
			forcePassthrough = true
		}
		out = append(out, arg)
	}
	return engine.PrepareResult{NormalizedArgs: out, ForcePassthrough: forcePassthrough}
}

func (gitFetchFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Type != engine.EventExit {
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := mem.Joined()
	if ev.ExitCode != 0 {
		return flushRawOrIgnore(raw)
	}
	if strings.TrimSpace(raw) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	if summary, ok := summarizeGitFetch(raw); ok {
		return engine.Decision{Action: engine.ActionFlush, Output: summary}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: raw}
}

func isGitFetchDetailArg(arg string) bool {
	switch arg {
	case "--verbose", "-v", "--dry-run", "-n", "--porcelain":
		return true
	default:
		return false
	}
}

func summarizeGitFetch(raw string) (string, bool) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	summary := gitFetchSummary{}
	sawClassifiedUpdate := false
	for _, line := range lines {
		classified, ok := classifyGitFetchLine(strings.TrimSpace(line), &summary)
		if !ok {
			return "", false
		}
		if classified {
			sawClassifiedUpdate = true
		}
	}
	if !sawClassifiedUpdate {
		return "", false
	}
	return renderGitFetchSummary(summary), true
}

func classifyGitFetchLine(trimmed string, summary *gitFetchSummary) (bool, bool) {
	if trimmed == "" || strings.HasPrefix(trimmed, "From ") {
		return false, true
	}
	if !strings.Contains(trimmed, "->") {
		return false, false
	}
	summary.refUpdates++
	if strings.Contains(trimmed, "[new branch]") {
		summary.newBranches++
	}
	if strings.Contains(trimmed, "[new tag]") {
		summary.newTags++
	}
	if strings.Contains(trimmed, "[new ref]") {
		summary.newRefs++
	}
	if strings.Contains(trimmed, "forced update") || strings.HasPrefix(trimmed, "+") {
		summary.forced++
	}
	return true, true
}

func renderGitFetchSummary(summary gitFetchSummary) string {
	base := fmt.Sprintf("git fetch: ok %d %s", summary.refUpdates, pluralize(summary.refUpdates, "ref update", "ref updates"))
	details := make([]string, 0, 4)
	if summary.newBranches > 0 {
		details = append(details, fmt.Sprintf("%d %s", summary.newBranches, pluralize(summary.newBranches, "new branch", "new branches")))
	}
	if summary.newTags > 0 {
		details = append(details, fmt.Sprintf("%d %s", summary.newTags, pluralize(summary.newTags, "new tag", "new tags")))
	}
	if summary.newRefs > 0 {
		details = append(details, fmt.Sprintf("%d %s", summary.newRefs, pluralize(summary.newRefs, "new ref", "new refs")))
	}
	if summary.forced > 0 {
		details = append(details, fmt.Sprintf("%d %s", summary.forced, pluralize(summary.forced, "forced update", "forced updates")))
	}
	if len(details) == 0 {
		return base + "\n"
	}
	return base + " (" + strings.Join(details, ", ") + ")\n"
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
