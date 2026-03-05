package gitfilters

import (
	"fmt"
	"strings"

	"go-command-compression-proxy/internal/engine"
)

// NewGitCommitFilter compacts git commit write-path output.
func NewGitCommitFilter() engine.ToolFilter { return gitCommitFilter{} }

type gitCommitFilter struct{}

func (gitCommitFilter) Tool() string        { return "git commit" }
func (gitCommitFilter) Aliases() []string   { return nil }
func (gitCommitFilter) MaskingHorizon() int { return 0 }
func (gitCommitFilter) ContextKey(ev engine.Event) string {
	return sharedContextKey(ev)
}

func (gitCommitFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}

func (gitCommitFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	return processExit(ev, mem, summarizeCommitSuccess)
}

func summarizeCommitSuccess(raw string) string {
	rawLower := strings.ToLower(strings.TrimSpace(raw))
	hash := extractCommitHash(raw)
	files, adds, dels := extractChangeSummary(raw)
	if files > 0 && hash != "" {
		return fmt.Sprintf("git commit: ok %s %d files +%d -%d\n", hash, files, adds, dels)
	}
	if files > 0 {
		return fmt.Sprintf("git commit: ok %d files +%d -%d\n", files, adds, dels)
	}
	if hash != "" {
		return fmt.Sprintf("git commit: ok %s\n", hash)
	}
	if strings.Contains(rawLower, "nothing to commit") {
		return "git commit: ok (nothing to commit)\n"
	}
	return "git commit: ok\n"
}

func extractCommitHash(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") {
			continue
		}
		end := strings.Index(line, "]")
		if end <= 0 {
			continue
		}
		chunk := strings.TrimPrefix(line[:end], "[")
		parts := strings.Fields(chunk)
		if len(parts) < 2 {
			continue
		}
		hash := parts[len(parts)-1]
		if len(hash) < 7 {
			continue
		}
		if len(hash) > 7 {
			hash = hash[:7]
		}
		return hash
	}
	return ""
}
