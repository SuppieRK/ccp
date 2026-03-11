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

type lsCompactor struct{}

type file struct {
	name string
	size string
}

// NewLSCompactor returns the built-in ls tool filter.
func NewLSCompactor() engine.ToolFilter { return lsCompactor{} }

func (lsCompactor) Tool() string      { return "ls" }
func (lsCompactor) Aliases() []string { return nil }
func (lsCompactor) Prepare(args []string) engine.PrepareResult {
	hasLong, flags, paths := parseLSPrepareArgs(args)
	if !hasLong {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}
	return engine.PrepareResult{NormalizedArgs: normalizeLSPrepareArgs(flags, paths)}
}

func parseLSPrepareArgs(args []string) (bool, []string, []string) {
	hasLong := false
	flags := make([]string, 0, len(args))
	paths := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			if !strings.HasPrefix(arg, "--") && strings.ContainsRune(strings.TrimPrefix(arg, "-"), 'l') {
				hasLong = true
			}
			flags = append(flags, arg)
			continue
		}
		paths = append(paths, arg)
	}
	return hasLong, flags, paths
}

func normalizeLSPrepareArgs(flags, paths []string) []string {
	normalized := []string{"-la"}
	for _, flag := range flags {
		normalized = append(normalized, normalizeLSFlag(flag)...)
	}
	if len(paths) > 0 {
		return append(normalized, paths...)
	}
	return append(normalized, ".")
}

func normalizeLSFlag(flag string) []string {
	if strings.HasPrefix(flag, "--") {
		if flag == "--all" {
			return nil
		}
		return []string{flag}
	}
	stripped := strings.TrimPrefix(flag, "-")
	extra := make([]rune, 0, len(stripped))
	for _, r := range stripped {
		if r == 'l' || r == 'a' || r == 'h' {
			continue
		}
		extra = append(extra, r)
	}
	if len(extra) == 0 {
		return nil
	}
	return []string{"-" + string(extra)}
}

func (lsCompactor) ContextKey(ev engine.Event) string {
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}

func (lsCompactor) MaskingHorizon() int { return 0 }

func (lsCompactor) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	// Do no harm: keep stderr raw for ls so system diagnostics are never compacted.
	if d, ok := stderrImmediateOrIgnore(ev, nil); ok {
		return d
	}

	if ev.Type != engine.EventEOF {
		return engine.Decision{Action: engine.ActionCollect}
	}
	raw := mem.Joined()
	if strings.TrimSpace(raw) == "" {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	parsed := parseLSOutput(raw)
	if parsed.onlyErrors() {
		return engine.Decision{Action: engine.ActionFlush, Output: strings.Join(parsed.errorLines, "\n") + "\n"}
	}

	dirs := parsed.dirs
	files := parsed.files
	if parsed.requiresShortListingEnrichment() {
		if fsDirs, fsFiles, ok := enrichShortListingFromFS(ev.Dispatch, parsed.sectionEntries, parsed.sectionOrder); ok {
			dirs = fsDirs
			files = fsFiles
		} else {
			out := renderRawShortListing(parsed.shortEntries, parsed.sectionEntries, parsed.sectionOrder)
			if strings.TrimSpace(out) == "" {
				return engine.Decision{Action: engine.ActionFlush, Output: "(empty)\n"}
			}
			return engine.Decision{Action: engine.ActionFlush, Output: out}
		}
	}

	if len(dirs) == 0 && len(files) == 0 {
		return engine.Decision{Action: engine.ActionFlush, Output: "(empty)\n"}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: renderLSListing(dirs, files)}
}

type lsParsedOutput struct {
	dirs           []string
	files          []file
	errorLines     []string
	shortEntries   []string
	sectionEntries map[string][]string
	sectionOrder   []string
	currentSection string
}

func parseLSOutput(raw string) lsParsedOutput {
	parsed := lsParsedOutput{
		sectionEntries: map[string][]string{},
	}
	for _, line := range filtercommon.NonEmptyLines(raw) {
		parsed.consumeLine(line)
	}
	return parsed
}

func (p *lsParsedOutput) consumeLine(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "." || trimmed == ".." {
		return
	}
	if strings.HasPrefix(trimmed, "ls:") {
		// Preserve native ls failure diagnostics when stderr is not available.
		p.errorLines = append(p.errorLines, trimmed)
		return
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "total ") || strings.HasPrefix(strings.ToLower(trimmed), "insgesamt ") {
		return
	}
	if strings.HasSuffix(trimmed, ":") {
		p.currentSection = strings.TrimSuffix(trimmed, ":")
		if _, ok := p.sectionEntries[p.currentSection]; !ok {
			p.sectionEntries[p.currentSection] = make([]string, 0, 8)
			p.sectionOrder = append(p.sectionOrder, p.currentSection)
		}
		return
	}
	parts := strings.Fields(line)
	if len(parts) < 9 {
		p.appendShortEntry(trimmed)
		return
	}
	p.appendLongEntry(parts)
}

func (p *lsParsedOutput) appendShortEntry(trimmed string) {
	if p.currentSection != "" {
		p.sectionEntries[p.currentSection] = append(p.sectionEntries[p.currentSection], trimmed)
		return
	}
	p.shortEntries = append(p.shortEntries, trimmed)
}

func (p *lsParsedOutput) appendLongEntry(parts []string) {
	name := strings.Join(parts[8:], " ")
	if name == "." || name == ".." {
		return
	}
	mode := parts[0]
	switch {
	case strings.HasPrefix(mode, "d"):
		p.dirs = append(p.dirs, name)
	case strings.HasPrefix(mode, "-"), strings.HasPrefix(mode, "l"):
		size, _ := strconv.ParseUint(parts[4], 10, 64)
		p.files = append(p.files, file{name: name, size: humanSize(size)})
	}
}

func (p *lsParsedOutput) onlyErrors() bool {
	return len(p.errorLines) > 0 && len(p.dirs) == 0 && len(p.files) == 0
}

func (p *lsParsedOutput) requiresShortListingEnrichment() bool {
	if len(p.dirs) > 0 || len(p.files) > 0 {
		return false
	}
	return len(p.shortEntries) > 0 || len(p.sectionEntries) > 0
}

func renderRawShortListing(shortEntries []string, sectionEntries map[string][]string, sectionOrder []string) string {
	var b strings.Builder
	if len(sectionEntries) == 0 {
		b.WriteString(strings.Join(shortEntries, " "))
		b.WriteString("\n")
		return b.String()
	}
	for _, sec := range sectionOrder {
		entries := sectionEntries[sec]
		if len(entries) == 0 {
			continue
		}
		b.WriteString(sec)
		b.WriteString(": ")
		b.WriteString(strings.Join(entries, " "))
		b.WriteString("\n")
	}
	return b.String()
}

func renderLSListing(dirs []string, files []file) string {
	var b strings.Builder
	for _, dir := range dirs {
		b.WriteString(dir)
		b.WriteString("/\n")
	}
	for _, f := range files {
		b.WriteString(f.name)
		b.WriteString("  ")
		b.WriteString(f.size)
		b.WriteString("\n")
	}

	totalEntries := len(files) + len(dirs)
	if totalEntries > 2 {
		b.WriteString("\n")
		_, _ = fmt.Fprintf(&b, "summary: %d files, %d dirs", len(files), len(dirs))
		b.WriteString("\n")
	}

	return b.String()
}

func enrichShortListingFromFS(dispatch string, sectionEntries map[string][]string, sectionOrder []string) ([]string, []file, bool) {
	targets, recursive := lsTargetsFromDispatch(dispatch)
	if len(targets) == 0 {
		targets = []string{"."}
	}
	collector := newLSFSCollector()
	if len(sectionEntries) > 0 {
		collector.collectFromSections(sectionEntries, sectionOrder)
	} else {
		collector.collectFromTargets(targets, recursive)
	}
	if len(collector.dirs) == 0 && len(collector.files) == 0 {
		return nil, nil, false
	}
	return collector.sorted()
}

type lsFSCollector struct {
	seenDirs  map[string]struct{}
	seenFiles map[string]struct{}
	dirs      []string
	files     []file
}

func newLSFSCollector() *lsFSCollector {
	return &lsFSCollector{
		seenDirs:  map[string]struct{}{},
		seenFiles: map[string]struct{}{},
		dirs:      make([]string, 0, 16),
		files:     make([]file, 0, 32),
	}
}

func (c *lsFSCollector) collectFromSections(sectionEntries map[string][]string, sectionOrder []string) {
	for _, section := range sectionOrder {
		for _, entry := range sectionEntries[section] {
			full := filepath.Join(section, entry)
			st, err := os.Lstat(full)
			if err != nil {
				continue
			}
			if st.IsDir() {
				c.addDir(filepath.Base(entry))
				continue
			}
			c.addFile(filepath.Base(entry), uint64(st.Size()))
		}
	}
}

func (c *lsFSCollector) collectFromTargets(targets []string, recursive bool) {
	for _, target := range targets {
		st, err := os.Lstat(target)
		if err != nil {
			continue
		}
		if !st.IsDir() {
			c.addFile(filepath.Base(target), uint64(st.Size()))
			continue
		}
		if recursive {
			c.walkRecursiveTarget(target)
			continue
		}
		c.collectDirectoryEntries(target)
	}
}

func (c *lsFSCollector) walkRecursiveTarget(target string) {
	_ = filepath.WalkDir(target, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || path == target {
			return nil
		}
		name := filepath.Base(path)
		if isLSDotEntry(name) {
			return nil
		}
		if d.IsDir() {
			c.addDir(name)
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		c.addFile(name, uint64(info.Size()))
		return nil
	})
}

func (c *lsFSCollector) collectDirectoryEntries(target string) {
	entries, err := os.ReadDir(target)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if isLSDotEntry(name) {
			continue
		}
		if entry.IsDir() {
			c.addDir(name)
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		c.addFile(name, uint64(info.Size()))
	}
}

func (c *lsFSCollector) addDir(name string) {
	name = strings.TrimSpace(name)
	if name == "" || isLSDotEntry(name) {
		return
	}
	if _, ok := c.seenDirs[name]; ok {
		return
	}
	c.seenDirs[name] = struct{}{}
	c.dirs = append(c.dirs, name)
}

func (c *lsFSCollector) addFile(name string, size uint64) {
	name = strings.TrimSpace(name)
	if name == "" || isLSDotEntry(name) {
		return
	}
	if _, ok := c.seenFiles[name]; ok {
		return
	}
	c.seenFiles[name] = struct{}{}
	c.files = append(c.files, file{name: name, size: humanSize(size)})
}

func isLSDotEntry(name string) bool {
	return name == "." || name == ".."
}

func (c *lsFSCollector) sorted() ([]string, []file, bool) {
	sort.Strings(c.dirs)
	sort.Slice(c.files, func(i, j int) bool {
		return c.files[i].name < c.files[j].name
	})
	return c.dirs, c.files, true
}

func lsTargetsFromDispatch(dispatch string) ([]string, bool) {
	dispatch = strings.TrimSpace(dispatch)
	if dispatch == "" {
		return nil, false
	}
	parts := strings.Fields(dispatch)
	if parts[0] == "ls" {
		parts = parts[1:]
	}
	targets := make([]string, 0, len(parts))
	recursive := false
	for _, part := range parts {
		if strings.HasPrefix(part, "-") {
			flags := strings.TrimPrefix(part, "-")
			if strings.ContainsRune(flags, 'R') {
				recursive = true
			}
			continue
		}
		targets = append(targets, part)
	}
	return targets, recursive
}

func humanSize(size uint64) string {
	if size >= 1_048_576 {
		return fmt.Sprintf("%.1fM", float64(size)/1_048_576.0)
	}
	if size >= 1024 {
		return fmt.Sprintf("%.1fK", float64(size)/1024.0)
	}
	return fmt.Sprintf("%dB", size)
}
