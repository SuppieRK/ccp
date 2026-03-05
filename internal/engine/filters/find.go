package filters

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

const (
	defaultFindMaxResults = 200
)

type findFilter struct{}

// NewFindFilter returns the built-in find tool filter.
func NewFindFilter() engine.ToolFilter { return findFilter{} }

func (findFilter) Tool() string { return "find" }

func (findFilter) Aliases() []string { return nil }

func (findFilter) Prepare(args []string) engine.PrepareResult {
	parsed := parseFindArgs(args)
	dispatch := buildFindDispatch(parsed)

	result := engine.PrepareResult{
		NormalizedArgs: parsed.normalized,
		DispatchKey:    dispatch,
	}

	if canSubstituteFindWithFD(parsed) {
		fdArgs := toFDArgs(parsed)
		if len(fdArgs) > 0 {
			result.PreferredSubstitution = "fd"
			result.PreferredArgs = fdArgs
			result.FallbackArgs = parsed.normalized
		}
	}

	return result
}

func (findFilter) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}

func (findFilter) MaskingHorizon() int { return 1024 }

func (findFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if d, ok := stderrImmediateOrIgnore(ev, nil); ok {
		return d
	}

	cfg := parseFindDispatch(ev.Dispatch)

	switch ev.Type {
	case engine.EventTick:
		if cfg.heartbeat && mem.Len() > 0 {
			return engine.Decision{Action: engine.ActionFlush, Output: fmt.Sprintf("[find] scanned %d paths...\n", mem.Len())}
		}
		return engine.Decision{Action: engine.ActionCollect}
	case engine.EventExit:
		raw := mem.Joined()
		if strings.TrimSpace(raw) == "" {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		compacted, ok := compactFindOutput(raw, cfg)
		if !ok {
			return engine.Decision{Action: engine.ActionFlush, Output: raw}
		}
		if strings.TrimSpace(compacted) == "" {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: compacted}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

type findDispatch struct {
	pattern       string
	fileType      string
	maxResults    int
	root          string
	includeHidden bool
	heartbeat     bool
}

type parsedFindArgs struct {
	normalized    []string
	root          string
	pattern       string
	fileType      string
	maxResults    int
	includeHidden bool
	heartbeat     bool
}

func parseFindArgs(args []string) parsedFindArgs {
	state := findArgParseState{
		normalized: make([]string, 0, len(args)),
		paths:      make([]string, 0, 2),
		pattern:    "*",
		fileType:   "f",
		maxResults: defaultFindMaxResults,
		// find includes hidden paths by default; keep behavior semantically aligned.
		includeHidden: true,
	}
	state.consumeArgs(args)
	return state.build()
}

func buildFindDispatch(parsed parsedFindArgs) string {
	return strings.Join([]string{
		"find",
		"pattern=" + parsed.pattern,
		"type=" + parsed.fileType,
		"max=" + strconv.Itoa(parsed.maxResults),
		"root=" + parsed.root,
		"hidden=" + boolAs01(parsed.includeHidden),
		"heartbeat=" + boolAs01(parsed.heartbeat),
	}, "|")
}

func parseFindDispatch(dispatch string) findDispatch {
	cfg := findDispatch{
		pattern:    "*",
		fileType:   "f",
		maxResults: defaultFindMaxResults,
		root:       ".",
		// Historical rows may omit hidden=1, but runtime dispatch always sets it.
		includeHidden: true,
	}
	for k, v := range filtercommon.ParseDispatchMap(dispatch) {
		switch k {
		case "pattern":
			if v != "" {
				cfg.pattern = normalizeFindPattern(v)
			}
		case "type":
			cfg.fileType = normalizeFindType(v)
		case "max":
			cfg.maxResults = filtercommon.ParsePositiveInt(v, cfg.maxResults)
		case "root":
			if strings.TrimSpace(v) != "" {
				cfg.root = v
			}
		case "hidden":
			cfg.includeHidden = filtercommon.ParseBool01(v)
		case "heartbeat":
			cfg.heartbeat = filtercommon.ParseBool01(v)
		}
	}
	if cfg.pattern == "." {
		cfg.pattern = "*"
	}
	return cfg
}

type findEntry struct {
	dir  string
	name string
}

func compactFindOutput(raw string, cfg findDispatch) (string, bool) {
	lines := filtercommon.NonEmptyLines(raw)
	state := findCompactState{
		cfg:             cfg,
		rootClean:       filepath.Clean(cfg.root),
		typeByPathCache: map[string]bool{},
		files:           make([]findEntry, 0, len(lines)),
		dirs:            make([]string, 0, len(lines)),
	}
	for _, line := range lines {
		if !state.consumeLine(line) {
			return "", false
		}
	}
	if cfg.fileType == "d" {
		return renderDirectoryList(state.dirs, cfg.maxResults), true
	}
	return renderGroupedFiles(state.files, cfg.maxResults), true
}

type findArgParseState struct {
	normalized []string
	paths      []string

	pattern       string
	fileType      string
	maxResults    int
	includeHidden bool
	heartbeat     bool
	seenExpr      bool
}

func (s *findArgParseState) consumeArgs(args []string) {
	if len(args) == 0 {
		args = []string{"."}
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		advance, handled := s.consumeControlArg(arg, args, i)
		if handled {
			i += advance
			continue
		}
		if s.consumePathArg(arg) {
			continue
		}
		i += s.consumeExpressionArg(arg, args, i)
	}
}

func (s *findArgParseState) consumeControlArg(arg string, args []string, idx int) (int, bool) {
	switch arg {
	case "--all":
		s.includeHidden = true
		return 0, true
	case "--heartbeat":
		s.heartbeat = true
		return 0, true
	case "--max-results", "--max_results":
		if idx+1 < len(args) {
			s.maxResults = filtercommon.ParsePositiveInt(args[idx+1], s.maxResults)
			return 1, true
		}
		return 0, true
	}
	if strings.HasPrefix(arg, "--max-results=") || strings.HasPrefix(arg, "--max_results=") {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 {
			s.maxResults = filtercommon.ParsePositiveInt(parts[1], s.maxResults)
		}
		return 0, true
	}
	return 0, false
}

func (s *findArgParseState) consumePathArg(arg string) bool {
	if s.seenExpr || arg == "!" || arg == "(" || arg == ")" || strings.HasPrefix(arg, "-") {
		return false
	}
	s.paths = append(s.paths, arg)
	s.normalized = append(s.normalized, arg)
	return true
}

func (s *findArgParseState) consumeExpressionArg(arg string, args []string, idx int) int {
	s.seenExpr = true
	s.normalized = append(s.normalized, arg)

	if idx+1 >= len(args) {
		return 0
	}
	next := args[idx+1]

	switch arg {
	case "-name", "-iname", "-path", "-ipath":
		s.pattern = normalizeFindPattern(next)
		s.normalized = append(s.normalized, next)
		return 1
	case "-type":
		s.fileType = normalizeFindType(next)
		s.normalized = append(s.normalized, next)
		return 1
	default:
		return 0
	}
}

func (s *findArgParseState) build() parsedFindArgs {
	if len(s.paths) == 0 {
		s.paths = []string{"."}
		s.normalized = append([]string{"."}, s.normalized...)
	}
	root := s.paths[0]
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	pattern := s.pattern
	if pattern == "" {
		pattern = "*"
	}
	return parsedFindArgs{
		normalized:    s.normalized,
		root:          root,
		pattern:       pattern,
		fileType:      s.fileType,
		maxResults:    s.maxResults,
		includeHidden: s.includeHidden,
		heartbeat:     s.heartbeat,
	}
}

func normalizeFindPattern(pattern string) string {
	if strings.TrimSpace(pattern) == "." {
		return "*"
	}
	return pattern
}

func normalizeFindType(fileType string) string {
	if strings.ToLower(strings.TrimSpace(fileType)) == "d" {
		return "d"
	}
	return "f"
}

type findCompactState struct {
	cfg             findDispatch
	rootClean       string
	typeByPathCache map[string]bool
	files           []findEntry
	dirs            []string
}

func (s *findCompactState) consumeLine(line string) bool {
	if strings.ContainsRune(line, '\x00') {
		return false
	}
	path := filepath.Clean(line)
	rel, err := filepath.Rel(s.rootClean, path)
	switch {
	case s.rootClean == "":
		rel = path
	case err != nil, rel == "..", strings.HasPrefix(rel, ".."+string(filepath.Separator)):
		rel = path
	}
	rel = toSlash(rel)
	if rel == "" || rel == "." {
		return true
	}
	if s.shouldSkip(rel) {
		return true
	}
	if s.cfg.fileType == "d" {
		s.dirs = append(s.dirs, rel)
		return true
	}
	dir := filepath.Dir(rel)
	if dir == "." {
		dir = ""
	}
	s.files = append(s.files, findEntry{
		dir:  toSlash(dir),
		name: filepath.Base(rel),
	})
	return true
}

func (s *findCompactState) shouldSkip(rel string) bool {
	isDir := inferIsDir(s.rootClean, rel, s.cfg.fileType, s.typeByPathCache)
	if s.cfg.fileType == "d" && !isDir {
		return true
	}
	if s.cfg.fileType != "d" && isDir {
		return true
	}
	if !s.cfg.includeHidden && hasHiddenSegment(rel) {
		return true
	}
	return false
}

func renderDirectoryList(dirs []string, maxResults int) string {
	if len(dirs) == 0 {
		return ""
	}
	seen := map[string]struct{}{}
	unique := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		dir = toSlash(strings.TrimSpace(dir))
		if dir == "" {
			continue
		}
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		unique = append(unique, dir)
	}
	sort.Strings(unique)
	dirs = unique
	shown := dirs
	if len(shown) > maxResults {
		shown = shown[:maxResults]
	}
	var b strings.Builder
	for _, d := range shown {
		b.WriteString(toSlash(strings.TrimSuffix(d, "/")))
		b.WriteString("/\n")
	}
	if len(dirs) > len(shown) {
		b.WriteString("+" + strconv.Itoa(len(dirs)-len(shown)) + " more\n")
	}
	return b.String()
}

func renderGroupedFiles(files []findEntry, maxResults int) string {
	if len(files) == 0 {
		return ""
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].dir == files[j].dir {
			return files[i].name < files[j].name
		}
		return files[i].dir < files[j].dir
	})

	shown := files
	if len(shown) > maxResults {
		shown = shown[:maxResults]
	}

	groups := map[string][]string{}
	dirOrder := make([]string, 0, len(shown))
	for _, f := range shown {
		if _, ok := groups[f.dir]; !ok {
			dirOrder = append(dirOrder, f.dir)
		}
		groups[f.dir] = append(groups[f.dir], f.name)
	}
	sort.Strings(dirOrder)

	var b strings.Builder
	for _, dir := range dirOrder {
		label := dir
		if label == "" {
			label = "."
		}
		b.WriteString(label)
		b.WriteString("/\n")
		names := groups[dir]
		sort.Strings(names)
		for _, name := range names {
			b.WriteString("  ")
			b.WriteString(name)
			b.WriteString("\n")
		}
	}

	if len(files) > len(shown) {
		b.WriteString("+")
		b.WriteString(strconv.Itoa(len(files) - len(shown)))
		b.WriteString(" more\n")
	}
	return b.String()
}

func inferIsDir(root string, rel string, fileType string, typeByPathCache map[string]bool) bool {
	if fileType == "d" {
		return true
	}
	if strings.HasSuffix(rel, "/") {
		return true
	}
	if cached, ok := typeByPathCache[rel]; ok {
		return cached
	}
	abs := rel
	if root != "" && !filepath.IsAbs(rel) {
		abs = filepath.Join(root, rel)
	}
	if st, err := os.Stat(abs); err == nil {
		isDir := st.IsDir()
		typeByPathCache[rel] = isDir
		return isDir
	}
	typeByPathCache[rel] = false
	return false
}

func hasHiddenSegment(path string) bool {
	for _, seg := range strings.Split(toSlash(path), "/") {
		seg = strings.TrimSpace(seg)
		if seg == "" || seg == "." || seg == ".." {
			continue
		}
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

func toSlash(s string) string {
	return strings.ReplaceAll(s, "\\", "/")
}

func canSubstituteFindWithFD(parsed parsedFindArgs) bool {
	for _, arg := range parsed.normalized {
		if arg == "-exec" || arg == "-prune" || arg == "-delete" || arg == "-o" || arg == "-or" {
			return false
		}
	}
	return true
}

func toFDArgs(parsed parsedFindArgs) []string {
	args := make([]string, 0, len(parsed.normalized)+8)
	// Keep fd substitution semantically closer to find:
	// include hidden files and do not apply ignore rules.
	args = append(args, "--color=never", "--strip-cwd-prefix", "--hidden", "--no-ignore")
	if parsed.fileType == "d" {
		args = append(args, "--type", "d")
	} else {
		args = append(args, "--type", "f")
	}
	pattern := parsed.pattern
	if pattern == "" {
		pattern = "."
	}
	args = append(args, pattern)
	if parsed.root != "" {
		args = append(args, parsed.root)
	}
	return args
}
