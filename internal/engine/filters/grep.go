package filters

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

const (
	defaultGrepMaxResults = 50
	defaultGrepMaxContext = 120
	grepFlagRegexp        = "--regexp"
	grepFlagMaxCount      = "--max-count"
	grepFlagMaxResults    = "--max-results"
	grepFlagMaxResultsAlt = "--max_results"
)

type grepFilter struct{}

// NewGrepFilter returns the built-in grep/rg tool filter.
func NewGrepFilter() engine.ToolFilter { return grepFilter{} }

func (grepFilter) Tool() string { return "grep" }

func (grepFilter) Aliases() []string { return []string{"rg"} }

func (grepFilter) Prepare(args []string) engine.PrepareResult {
	contextOnly := filtercommon.HasAnyFlag(args, "--context-only", "--context_only")
	maxResults := filtercommon.ParsePositiveIntOptionAny(args, defaultGrepMaxResults, "-m", grepFlagMaxCount, grepFlagMaxResults, grepFlagMaxResultsAlt)
	dispatch := "grep|max=" + strconv.Itoa(maxResults) + "|context_only=" + boolAs01(contextOnly)

	if unsafeBRETranslation(args) {
		normalized := ensurePathDefault(args)
		return engine.PrepareResult{
			NormalizedArgs:   normalized,
			DispatchKey:      dispatch,
			Ambiguous:        true,
			Reason:           "unsafe grep regex translation for preferred substitution",
			ForcePassthrough: true,
		}
	}

	rgArgs := normalizeForRG(args)
	grepArgs := normalizeForGrepFallback(args)
	return engine.PrepareResult{
		NormalizedArgs:        rgArgs,
		DispatchKey:           dispatch,
		PreferredSubstitution: "rg",
		PreferredArgs:         rgArgs,
		FallbackArgs:          grepArgs,
	}
}

func (grepFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}

func (grepFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	// Keep command diagnostics untouched.
	if d, ok := stderrImmediateOrIgnore(ev, normalizeGrepErrorLine); ok {
		return d
	}
	if d, ok := collectOnLineTickEOF(ev); ok {
		return d
	}
	switch ev.Type {
	case engine.EventExit:
		return processGrepExit(ev, mem.Joined())
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

func processGrepExit(ev engine.Event, raw string) engine.Decision {
	cfg := parseGrepDispatch(ev.Dispatch)
	if strings.TrimSpace(raw) == "" {
		return grepNoMatchesDecision(ev.ExitCode)
	}
	out, ok := compactGrepOutput(raw, cfg)
	if !ok {
		return engine.Decision{Action: engine.ActionFlush, Output: raw}
	}
	if out == "" {
		return grepNoMatchesDecision(ev.ExitCode)
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}

func grepNoMatchesDecision(exitCode int) engine.Decision {
	if exitCode != 0 {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	return engine.Decision{Action: engine.ActionIgnore}
}

func (grepFilter) MaskingHorizon() int { return 1024 }

type grepDispatch struct {
	maxResults  int
	contextOnly bool
}

func parseGrepDispatch(dispatch string) grepDispatch {
	cfg := grepDispatch{maxResults: defaultGrepMaxResults}
	for k, v := range filtercommon.ParseDispatchMap(dispatch) {
		switch k {
		case "max":
			cfg.maxResults = filtercommon.ParsePositiveInt(v, cfg.maxResults)
		case "context_only":
			cfg.contextOnly = filtercommon.ParseBool01(v)
		}
	}
	return cfg
}

type grepMatch struct {
	file string
	line int
	text string
}

func compactGrepOutput(raw string, cfg grepDispatch) (string, bool) {
	matches := make([]grepMatch, 0, 64)
	for _, line := range filtercommon.NonEmptyLines(raw) {
		file, ln, text, ok := parseGrepLine(line)
		if !ok {
			return "", false
		}
		if cfg.contextOnly {
			text = filtercommon.TruncateWithSuffix(strings.TrimSpace(text), defaultGrepMaxContext-3, "...")
		}
		matches = append(matches, grepMatch{file: file, line: ln, text: text})
	}
	if len(matches) == 0 {
		return "", true
	}

	order, byFile, consumed := groupGrepMatches(matches, cfg.maxResults)
	return renderGroupedGrepMatches(order, byFile, len(matches)-consumed), true
}

type grepGroupedMatches struct {
	file    string
	matches []grepMatch
}

func groupGrepMatches(matches []grepMatch, maxResults int) ([]string, map[string]*grepGroupedMatches, int) {
	order := make([]string, 0, len(matches))
	byFile := map[string]*grepGroupedMatches{}
	consumed := 0
	for _, m := range matches {
		if consumed >= maxResults {
			break
		}
		consumed++
		if _, ok := byFile[m.file]; !ok {
			byFile[m.file] = &grepGroupedMatches{file: m.file}
			order = append(order, m.file)
		}
		byFile[m.file].matches = append(byFile[m.file].matches, m)
	}
	sort.Strings(order)
	return order, byFile, consumed
}

func renderGroupedGrepMatches(order []string, byFile map[string]*grepGroupedMatches, remaining int) string {
	var b strings.Builder
	for _, file := range order {
		group := byFile[file]
		b.WriteString(group.file)
		b.WriteString(" (")
		b.WriteString(strconv.Itoa(len(group.matches)))
		b.WriteString(" matches)\n")
		for _, m := range group.matches {
			b.WriteString("  ")
			b.WriteString(strconv.Itoa(m.line))
			b.WriteString(": ")
			b.WriteString(m.text)
			b.WriteString("\n")
		}
	}
	if remaining > 0 {
		b.WriteString("... +")
		b.WriteString(strconv.Itoa(remaining))
		b.WriteString(" more matches\n")
	}
	return b.String()
}

func parseGrepLine(line string) (string, int, string, bool) {
	parts := strings.Split(line, ":")
	if len(parts) < 3 {
		return "", 0, "", false
	}
	for i := 1; i < len(parts)-1; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			continue
		}
		file := strings.Join(parts[:i], ":")
		if strings.TrimSpace(file) == "" {
			continue
		}
		return file, n, strings.Join(parts[i+1:], ":"), true
	}
	return "", 0, "", false
}

func normalizeForRG(args []string) []string {
	// Keep rg recursive-by-default (intent-based divergence from classic grep defaults).
	out := []string{"--no-heading", "--with-filename", "--line-number", "--color=never"}
	out = append(out, normalizedSearchArgs(args, true)...)
	return out
}

func normalizeForGrepFallback(args []string) []string {
	out := []string{"-r", "-H", "-n"}
	out = append(out, normalizedSearchArgs(args, false)...)
	return out
}

func normalizedSearchArgs(args []string, forRG bool) []string {
	out := make([]string, 0, len(args)+1)
	expectValue := false
	seenPositional := 0
	for _, arg := range args {
		out, expectValue, seenPositional = appendNormalizedSearchArg(out, arg, forRG, expectValue, seenPositional)
	}
	out = ensurePathDefault(out)
	return out
}

func appendNormalizedSearchArg(out []string, arg string, forRG, expectValue bool, seenPositional int) ([]string, bool, int) {
	if expectValue {
		return append(out, translatePatternArg(arg)), false, seenPositional
	}
	switch arg {
	case "--context-only", "--context_only":
		return out, expectValue, seenPositional
	case "-r", "--recursive":
		if forRG {
			return out, expectValue, seenPositional
		}
		return append(out, arg), expectValue, seenPositional
	case "-e", grepFlagRegexp, "-A", "-B", "-C", "-m", grepFlagMaxCount, grepFlagMaxResults, grepFlagMaxResultsAlt, "-t", "--type":
		return append(out, arg), true, seenPositional
	}
	if strings.HasPrefix(arg, "-") {
		if strings.HasPrefix(arg, grepFlagMaxResults+"=") || strings.HasPrefix(arg, grepFlagMaxResultsAlt+"=") {
			return out, expectValue, seenPositional
		}
		return append(out, arg), expectValue, seenPositional
	}
	seenPositional++
	if seenPositional == 1 {
		return append(out, translatePatternArg(arg)), expectValue, seenPositional
	}
	return append(out, arg), expectValue, seenPositional
}

func ensurePathDefault(args []string) []string {
	positional := 0
	expectValue := false
	for _, arg := range args {
		if expectValue {
			expectValue = false
			continue
		}
		switch arg {
		case "-e", grepFlagRegexp, "-A", "-B", "-C", "-m", grepFlagMaxCount, grepFlagMaxResults, grepFlagMaxResultsAlt, "-t", "--type":
			expectValue = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		positional++
	}
	out := append([]string{}, args...)
	if positional < 2 {
		// Prevent stdin-hang behavior by always targeting current directory when path is omitted.
		return append(out, ".")
	}
	return out
}

func unsafeBRETranslation(args []string) bool {
	if filtercommon.HasAnyFlag(args, "-E", "--extended-regexp", "-P", "--perl-regexp") {
		return false
	}
	candidates := patternCandidates(args)
	for _, p := range candidates {
		if strings.Contains(p, `\(`) || strings.Contains(p, `\)`) || strings.Contains(p, `\+`) || strings.Contains(p, `\?`) || strings.Contains(p, `\{`) || strings.Contains(p, `\}`) {
			return true
		}
	}
	return false
}

func patternCandidates(args []string) []string {
	out := make([]string, 0, len(args))
	expectPattern := false
	for _, arg := range args {
		if expectPattern {
			out = append(out, arg)
			expectPattern = false
			continue
		}
		switch arg {
		case "-e", grepFlagRegexp:
			expectPattern = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if len(out) == 0 {
			out = append(out, arg)
		}
	}
	return out
}

func translatePatternArg(arg string) string {
	return strings.ReplaceAll(arg, `\|`, `|`)
}

func boolAs01(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

var rgMissingPathRE = regexp.MustCompile(`^rg:\s+(.+?):\s+IO error for operation on (.+?):\s+No such file or directory \(os error \d+\)\s*$`)
var osErrorSuffixRE = regexp.MustCompile(`\s+\(os error \d+\)$`)

func normalizeGrepErrorLine(line string) string {
	trimmed := strings.TrimRight(line, "\r\n")
	if m := rgMissingPathRE.FindStringSubmatch(trimmed); len(m) == 3 && m[1] == m[2] {
		return "grep: " + m[1] + ": No such file or directory\n"
	}
	trimmed = osErrorSuffixRE.ReplaceAllString(trimmed, "")
	if strings.HasPrefix(trimmed, "rg: ") {
		msg := strings.TrimPrefix(trimmed, "rg: ")
		return "grep: " + msg + "\n"
	}
	return trimmed + "\n"
}
