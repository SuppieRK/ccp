package gitfilters

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-command-compression-proxy/internal/engine"
)

// NewGitWorktreeFilter compacts safe git worktree list output.
func NewGitWorktreeFilter() engine.ToolFilter { return gitWorktreeFilter{} }

type gitWorktreeFilter struct{}

func (gitWorktreeFilter) Tool() string        { return "git worktree" }
func (gitWorktreeFilter) Aliases() []string   { return nil }
func (gitWorktreeFilter) MaskingHorizon() int { return 0 }
func (gitWorktreeFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}

func (gitWorktreeFilter) Prepare(args []string) engine.PrepareResult {
	if len(args) == 0 {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}
	if strings.ToLower(strings.TrimSpace(args[0])) != "list" {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}
	for _, arg := range args[1:] {
		if isGitWorktreePrecisionArg(arg) {
			return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
		}
	}
	return engine.PrepareResult{NormalizedArgs: args}
}

func (gitWorktreeFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
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
	if out, ok := compactGitWorktreeList(raw); ok {
		return engine.Decision{Action: engine.ActionFlush, Output: out}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: raw + "\n"}
}

func isGitWorktreePrecisionArg(arg string) bool {
	switch arg {
	case "--porcelain", "--verbose", "-v", "-z", "--expire":
		return true
	default:
		return false
	}
}

func compactGitWorktreeList(raw string) (string, bool) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		item, ok := parseGitWorktreeListLine(line)
		if !ok {
			return "", false
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return "", false
	}
	return strings.Join(items, "\n") + "\n", true
}

func parseGitWorktreeListLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", false
	}
	openBracket := strings.LastIndex(trimmed, "[")
	closeBracket := strings.LastIndex(trimmed, "]")
	if openBracket < 0 || closeBracket <= openBracket {
		return "", false
	}
	branch := strings.TrimSpace(trimmed[openBracket+1 : closeBracket])
	prefix := strings.TrimSpace(trimmed[:openBracket])
	fields := strings.Fields(prefix)
	if len(fields) < 2 {
		return "", false
	}
	hash := fields[len(fields)-1]
	path := strings.Join(fields[:len(fields)-1], " ")
	if path == "" || hash == "" || branch == "" {
		return "", false
	}
	return fmt.Sprintf("%s %s [%s]", shortenWorktreePath(path), hash, branch), true
}

func shortenWorktreePath(path string) string {
	cwd, err := os.Getwd()
	if err != nil || strings.TrimSpace(cwd) == "" {
		return path
	}
	resolvedCWD := resolvedWorktreePath(cwd)
	resolvedPath := resolvedWorktreePath(path)
	rel, err := filepath.Rel(resolvedCWD, resolvedPath)
	if err != nil {
		return path
	}
	rel = filepath.Clean(rel)
	if rel == "." {
		return rel
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || !filepath.IsAbs(path) {
		if len(rel) < len(path) {
			return filepath.ToSlash(rel)
		}
	}
	if len(rel) < len(path) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

func resolvedWorktreePath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || strings.TrimSpace(resolved) == "" {
		return filepath.Clean(path)
	}
	return filepath.Clean(resolved)
}
