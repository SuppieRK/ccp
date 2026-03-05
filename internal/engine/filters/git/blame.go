package gitfilters

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"go-command-compression-proxy/internal/engine"
)

// NewGitBlameFilter keeps safe passthrough behavior.
func NewGitBlameFilter() engine.ToolFilter { return gitBlameFilter{} }

type gitBlameFilter struct{}

func (gitBlameFilter) Tool() string      { return "git blame" }
func (gitBlameFilter) Aliases() []string { return nil }
func (gitBlameFilter) Prepare(args []string) engine.PrepareResult {
	for _, arg := range args {
		if arg == "--line-porcelain" {
			return engine.PrepareResult{NormalizedArgs: args}
		}
	}
	return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
}
func (gitBlameFilter) ContextKey(ev engine.Event) string { return sharedContextKey(ev) }
func (gitBlameFilter) MaskingHorizon() int               { return 0 }
func (gitBlameFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if ev.Stream == engine.StderrStream {
		if ev.Type == engine.EventLine {
			return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
		}
		return engine.Decision{Action: engine.ActionIgnore}
	}
	if ev.Type != engine.EventExit {
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := mem.Joined()
	if ev.ExitCode != 0 {
		return flushRawOrIgnore(raw)
	}
	if compacted := compactBlameLinePorcelain(raw); compacted != "" {
		return engine.Decision{Action: engine.ActionFlush, Output: compacted}
	}
	return flushRawOrIgnore(raw)
}

type blameEntry struct {
	hash          string
	line          string
	author        string
	authorTime    string
	authorTZ      string
	committer     string
	committerTime string
	committerTZ   string
	file          string
	code          string
}

const blameTimeFormat = "2006-01-02 15:04:05 -0700"

type blameParseState struct {
	current           blameEntry
	lastAuthor        string
	lastAuthorTime    string
	lastAuthorTZ      string
	lastCommitter     string
	lastCommitterTime string
	lastCommitterTZ   string
	lastFile          string
	lastHash          string
}

func (s *blameParseState) flushInto(entries *[]blameEntry) {
	if s.current.code == "" || s.current.file == "" || s.current.line == "" {
		s.current = blameEntry{}
		return
	}
	if s.current.author == "" {
		s.current.author = s.lastAuthor
	}
	if s.current.authorTime == "" {
		s.current.authorTime = s.lastAuthorTime
	}
	if s.current.authorTZ == "" {
		s.current.authorTZ = s.lastAuthorTZ
	}
	if s.current.committer == "" {
		s.current.committer = s.lastCommitter
	}
	if s.current.committerTime == "" {
		s.current.committerTime = s.lastCommitterTime
	}
	if s.current.committerTZ == "" {
		s.current.committerTZ = s.lastCommitterTZ
	}
	if s.current.hash == "" {
		s.current.hash = s.lastHash
	}
	*entries = append(*entries, s.current)
	s.current = blameEntry{}
}

func (s *blameParseState) consumeMetadataLine(line string) bool {
	switch {
	case strings.HasPrefix(line, "author "):
		s.current.author = strings.TrimSpace(strings.TrimPrefix(line, "author "))
		s.lastAuthor = s.current.author
		return true
	case strings.HasPrefix(line, "author-time "):
		s.current.authorTime = strings.TrimSpace(strings.TrimPrefix(line, "author-time "))
		s.lastAuthorTime = s.current.authorTime
		return true
	case strings.HasPrefix(line, "author-tz "):
		s.current.authorTZ = strings.TrimSpace(strings.TrimPrefix(line, "author-tz "))
		s.lastAuthorTZ = s.current.authorTZ
		return true
	case strings.HasPrefix(line, "committer "):
		s.current.committer = strings.TrimSpace(strings.TrimPrefix(line, "committer "))
		s.lastCommitter = s.current.committer
		return true
	case strings.HasPrefix(line, "committer-time "):
		s.current.committerTime = strings.TrimSpace(strings.TrimPrefix(line, "committer-time "))
		s.lastCommitterTime = s.current.committerTime
		return true
	case strings.HasPrefix(line, "committer-tz "):
		s.current.committerTZ = strings.TrimSpace(strings.TrimPrefix(line, "committer-tz "))
		s.lastCommitterTZ = s.current.committerTZ
		return true
	case strings.HasPrefix(line, "filename "):
		s.current.file = strings.TrimSpace(strings.TrimPrefix(line, "filename "))
		s.lastFile = s.current.file
		return true
	default:
		return false
	}
}

func compactBlameLinePorcelain(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")
	entries := make([]blameEntry, 0, 16)
	state := blameParseState{}

	for _, line := range lines {
		if isBlameHeader(line) {
			state.flushInto(&entries)
			parts := strings.Fields(line)
			h := parts[0]
			state.current = blameEntry{
				hash:          h,
				line:          parts[2],
				author:        state.lastAuthor,
				authorTime:    state.lastAuthorTime,
				authorTZ:      state.lastAuthorTZ,
				committer:     state.lastCommitter,
				committerTime: state.lastCommitterTime,
				committerTZ:   state.lastCommitterTZ,
				file:          state.lastFile,
			}
			if h != "" {
				state.lastHash = h
			}
			continue
		}
		if state.consumeMetadataLine(line) {
			continue
		}
		if strings.HasPrefix(line, "\t") {
			state.current.code = strings.TrimPrefix(line, "\t")
			state.flushInto(&entries)
		}
	}
	state.flushInto(&entries)
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("git blame: %d lines\n", len(entries)))
	for _, e := range entries {
		hash := e.hash
		if len(hash) > 8 {
			hash = hash[:8]
		}
		authorTime := normalizeBlameTime(e.authorTime, e.authorTZ)
		committerTime := normalizeBlameTime(e.committerTime, e.committerTZ)
		b.WriteString(fmt.Sprintf(
			"%s:%s author=%s @ %s committer=%s @ %s %s %s\n",
			e.file, e.line, e.author, authorTime, e.committer, committerTime, hash, e.code,
		))
	}
	return b.String()
}

func isBlameHeader(line string) bool {
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return false
	}
	if len(parts[0]) < 8 {
		return false
	}
	_, err1 := strconv.Atoi(parts[1])
	_, err2 := strconv.Atoi(parts[2])
	return err1 == nil && err2 == nil
}

func normalizeBlameTime(epoch, tz string) string {
	sec, err := strconv.ParseInt(strings.TrimSpace(epoch), 10, 64)
	if err != nil {
		return ""
	}
	ts := time.Unix(sec, 0)
	if strings.HasPrefix(tz, "+") || strings.HasPrefix(tz, "-") {
		if len(tz) == 5 {
			sign := 1
			if tz[0] == '-' {
				sign = -1
			}
			hh, hErr := strconv.Atoi(tz[1:3])
			mm, mErr := strconv.Atoi(tz[3:5])
			if hErr == nil && mErr == nil {
				offset := sign * ((hh * 60 * 60) + (mm * 60))
				return ts.In(time.FixedZone("UTC", offset)).Format(blameTimeFormat)
			}
		}
	}
	return ts.UTC().Format(blameTimeFormat)
}
