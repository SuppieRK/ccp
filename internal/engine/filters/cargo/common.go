package cargofilters

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

const (
	maxDiagnostics     = 20
	maxExamplesPerLint = 3
	cargoErrorPrefix   = "error:"
	cargoWarningPrefix = "warning:"
	cargoHelpPrefix    = "= help:"
)

var (
	progressRe      = regexp.MustCompile(`^\s*(Compiling|Downloading|Fresh|Finished|Checking)\s+`)
	testResultRe    = regexp.MustCompile(`^test result:\s+(ok|FAILED)\.\s+(\d+)\s+passed;\s+(\d+)\s+failed(?:;\s+(\d+)\s+ignored)?`)
	testFailPkgRe   = regexp.MustCompile(`^error:\s+test failed,\s+to rerun pass`)
	testFailedLine  = regexp.MustCompile(`^test ([^ ]+) \.\.\. FAILED$`)
	panicAtLine     = regexp.MustCompile(`^thread '[^']+' panicked at (.+):$`)
	assertSideRe    = regexp.MustCompile(`^(left|right):\s+`)
	docLineRefRe    = regexp.MustCompile(`^[^ ]+\.rs\s+-\s+\(line\s+\d+\)$`)
	failureMarkerRe = regexp.MustCompile(`(?i)(^---- .* ----$|^thread '.*' panicked at|^\s*failures:\s*$|\bFAILED\b|^error: test failed|panic:)`)
	diagnosticRe    = regexp.MustCompile(`(?i)(error(\[[^\]]+\])?:|warning:|panic:|^\s*-->|\.rs:\d+)`)
	lintRuleRe      = regexp.MustCompile(`clippy::[a-z0-9_-]+`)
	panicThreadIDRe = regexp.MustCompile(`^(thread '.*') \(\d+\) panicked at`)
)

type sectionSummary struct {
	name    string
	passed  int
	failed  int
	ignored int
}

type lintGroup struct {
	count    int
	examples []string
	seen     map[string]struct{}
	locs     []string
	seenLoc  map[string]struct{}
}

func compactTest(raw string) (string, bool) {
	if strings.ContainsRune(raw, '\x00') {
		return "", false
	}
	lines := filtercommon.NonEmptyLines(raw)
	if len(lines) == 0 {
		return "cargo test: ok\n", true
	}
	state := newCargoTestCompactState()
	for _, line := range lines {
		state.consumeLine(line)
	}
	if state.recognized == 0 {
		return "", false
	}
	state.ensureCurrentSection()
	if state.shouldDropPackageOnlyFailure() {
		return "", true
	}
	return state.render(), true
}

func compactBuildCheck(raw, label string) (string, bool) {
	if strings.ContainsRune(raw, '\x00') {
		return "", false
	}
	lines := filtercommon.NonEmptyLines(raw)
	if len(lines) == 0 {
		return label + ": ok\n", true
	}

	recognized := 0
	diag := make([]string, 0, 32)
	for _, line := range lines {
		norm := strings.TrimSpace(line)
		if norm == "" {
			continue
		}
		if isBuildCheckProgressOrSummaryLine(norm) {
			recognized++
			continue
		}
		if !isBuildCheckDiagnosticLine(norm) {
			continue
		}
		recognized++
		diag = append(diag, norm)
	}

	if recognized == 0 {
		return "", false
	}
	if len(diag) == 0 {
		return label + ": ok\n", true
	}

	orderedDiag := prioritizeDiagnostics(diag)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s: %d diagnostics\n", label, len(orderedDiag)))
	for i, line := range orderedDiag {
		if i >= maxDiagnostics {
			b.WriteString(fmt.Sprintf("... +%d more\n", len(orderedDiag)-maxDiagnostics))
			break
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String(), true
}

func isBuildCheckProgressOrSummaryLine(line string) bool {
	if progressRe.MatchString(line) {
		return true
	}
	return isBuildCheckCompilationSummary(line) || isBuildCheckWarningSummary(line)
}

func isBuildCheckCompilationSummary(line string) bool {
	if !strings.HasPrefix(line, cargoErrorPrefix) {
		return false
	}
	return strings.Contains(line, "could not compile") || strings.Contains(line, "aborting due to")
}

func isBuildCheckWarningSummary(line string) bool {
	if !strings.HasPrefix(line, cargoWarningPrefix) {
		return false
	}
	return strings.Contains(line, "generated") && strings.Contains(line, "warning")
}

func isBuildCheckDiagnosticLine(line string) bool {
	return diagnosticRe.MatchString(line) || docLineRefRe.MatchString(line)
}

func compactClippy(raw string) (string, bool) {
	if strings.ContainsRune(raw, '\x00') {
		return "", false
	}
	lines := filtercommon.NonEmptyLines(raw)
	if len(lines) == 0 {
		return "cargo clippy: ok\n", true
	}
	state := newClippyCompactState()
	for _, line := range lines {
		state.consumeLine(line)
	}
	state.flushPending()

	if state.recognized == 0 {
		return "", false
	}
	totalFindings := state.totalFindings()
	if totalFindings == 0 {
		return "cargo clippy: ok\n", true
	}
	return state.render(totalFindings), true
}

type clippyCompactState struct {
	recognized      int
	groups          map[string]*lintGroup
	order           []string
	currentRule     string
	pendingFinding  string
	pendingRule     string
	pendingLocation string
	pendingExtras   []string
}

func newClippyCompactState() *clippyCompactState {
	return &clippyCompactState{
		groups:        map[string]*lintGroup{},
		order:         make([]string, 0, 8),
		pendingExtras: make([]string, 0, 8),
	}
}

func (s *clippyCompactState) consumeLine(line string) {
	norm := strings.TrimSpace(line)
	if norm == "" {
		return
	}
	if s.consumeProgressLine(norm) {
		return
	}
	isFinding := isClippyFindingLine(norm)
	rule := extractLintRule(norm)
	if !isFinding && s.currentRule == "" && s.pendingFinding == "" {
		return
	}
	s.recognized++
	if isFinding {
		s.consumeFindingLine(norm, rule)
		return
	}
	s.consumeDetailLine(norm, rule)
}

func (s *clippyCompactState) consumeProgressLine(norm string) bool {
	if progressRe.MatchString(norm) {
		s.recognized++
		return true
	}
	if strings.HasPrefix(norm, cargoErrorPrefix) && (strings.Contains(norm, "could not compile") || strings.Contains(norm, "aborting due to")) {
		s.recognized++
		return true
	}
	return false
}

func isClippyFindingLine(norm string) bool {
	return strings.HasPrefix(norm, cargoWarningPrefix) ||
		strings.HasPrefix(norm, cargoErrorPrefix) ||
		strings.HasPrefix(norm, "warning[") ||
		strings.HasPrefix(norm, "error[")
}

func (s *clippyCompactState) consumeFindingLine(norm, rule string) {
	s.flushPending()
	if rule == "" {
		s.pendingFinding = norm
		s.pendingRule = ""
		s.pendingExtras = s.pendingExtras[:0]
		s.currentRule = ""
		return
	}
	s.addFinding(rule, norm)
}

func (s *clippyCompactState) consumeDetailLine(norm, rule string) {
	if s.pendingFinding != "" {
		s.consumePendingDetail(norm, rule)
		return
	}
	if strings.HasPrefix(norm, "--> ") {
		s.addLocation(s.currentRule, strings.TrimSpace(strings.TrimPrefix(norm, "--> ")))
		return
	}
	s.addExample(s.currentRule, norm)
}

func (s *clippyCompactState) consumePendingDetail(norm, rule string) {
	if rule != "" {
		s.pendingRule = rule
	}
	if strings.HasPrefix(norm, "--> ") {
		s.pendingLocation = strings.TrimSpace(strings.TrimPrefix(norm, "--> "))
		return
	}
	if strings.HasPrefix(norm, cargoHelpPrefix) && strings.Contains(norm, "for further information visit") {
		return
	}
	if strings.HasPrefix(norm, "= note:") || strings.HasPrefix(norm, cargoHelpPrefix) {
		s.pendingExtras = append(s.pendingExtras, norm)
	}
}

func (s *clippyCompactState) addGroup(rule string) *lintGroup {
	grp := s.groups[rule]
	if grp != nil {
		return grp
	}
	grp = &lintGroup{seen: map[string]struct{}{}, seenLoc: map[string]struct{}{}}
	s.groups[rule] = grp
	s.order = append(s.order, rule)
	return grp
}

func (s *clippyCompactState) addExample(rule, line string) {
	if rule == "" {
		return
	}
	grp := s.addGroup(rule)
	if _, exists := grp.seen[line]; exists {
		return
	}
	grp.seen[line] = struct{}{}
	grp.examples = append(grp.examples, line)
}

func (s *clippyCompactState) addFinding(rule, line string) {
	if rule == "" {
		rule = "clippy::unknown"
	}
	grp := s.addGroup(rule)
	grp.count++
	s.addExample(rule, line)
	s.currentRule = rule
}

func (s *clippyCompactState) addLocation(rule, loc string) {
	if rule == "" || strings.TrimSpace(loc) == "" {
		return
	}
	grp := s.addGroup(rule)
	if _, exists := grp.seenLoc[loc]; exists {
		return
	}
	grp.seenLoc[loc] = struct{}{}
	grp.locs = append(grp.locs, loc)
}

func (s *clippyCompactState) flushPending() {
	if s.pendingFinding == "" {
		return
	}
	rule := s.pendingRule
	if rule == "" {
		rule = "clippy::unknown"
	}
	s.addFinding(rule, s.pendingFinding)
	if s.pendingLocation != "" {
		s.addLocation(rule, s.pendingLocation)
	}
	for _, extra := range s.pendingExtras {
		s.addExample(rule, extra)
	}
	s.pendingFinding = ""
	s.pendingRule = ""
	s.pendingLocation = ""
	s.pendingExtras = s.pendingExtras[:0]
}

func (s *clippyCompactState) totalFindings() int {
	total := 0
	for _, g := range s.groups {
		total += g.count
	}
	return total
}

func (s *clippyCompactState) render(totalFindings int) string {
	sort.Strings(s.order)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("cargo clippy: %d findings across %d lint rules\n", totalFindings, len(s.order)))
	for _, rule := range s.order {
		grp := s.groups[rule]
		if grp == nil {
			continue
		}
		if len(grp.locs) > 0 {
			b.WriteString(fmt.Sprintf("- %s: %d (%s)\n", rule, grp.count, grp.locs[0]))
		} else {
			b.WriteString(fmt.Sprintf("- %s: %d\n", rule, grp.count))
		}
		examples := prioritizeClippyExamples(grp.examples)
		for i, line := range examples {
			if i >= maxExamplesPerLint {
				b.WriteString(fmt.Sprintf("  ... +%d more\n", len(examples)-maxExamplesPerLint))
				break
			}
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func prioritizeClippyExamples(lines []string) []string {
	primary := make([]string, 0, len(lines))
	notes := make([]string, 0, len(lines))
	helps := make([]string, 0, len(lines))
	other := make([]string, 0, len(lines))
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		lower := strings.ToLower(trim)
		switch {
		case strings.HasPrefix(trim, cargoErrorPrefix) || strings.HasPrefix(trim, "error[") || strings.HasPrefix(trim, cargoWarningPrefix) || strings.HasPrefix(trim, "warning["):
			primary = append(primary, line)
		case strings.HasPrefix(trim, "= note:"):
			notes = append(notes, line)
		case strings.HasPrefix(trim, cargoHelpPrefix):
			helps = append(helps, line)
		case strings.Contains(lower, "panic") || strings.Contains(lower, "failed"):
			primary = append(primary, line)
		default:
			other = append(other, line)
		}
	}
	out := make([]string, 0, len(lines))
	out = append(out, primary...)
	out = append(out, notes...)
	out = append(out, helps...)
	out = append(out, other...)
	return out
}

func extractLintRule(line string) string {
	m := lintRuleRe.FindString(line)
	if m == "" {
		return ""
	}
	return strings.ReplaceAll(m, "-", "_")
}

func normalizeCargoTestDetail(line string) string {
	if line == "" {
		return line
	}
	return panicThreadIDRe.ReplaceAllString(line, "$1 panicked at")
}

func collapseCargoTestFailureDetails(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if skipCargoFailureDetailLine(line) {
			continue
		}
		if m := testFailedLine.FindStringSubmatch(line); len(m) > 0 {
			summary, advance, assertDetails := parseCollapsedCargoFailure(m[1], lines, i)
			out = append(out, summary)
			out = append(out, assertDetails...)
			i = advance
			continue
		}
		out = append(out, line)
	}
	return out
}

type cargoTestCompactState struct {
	recognized     int
	sectionOrder   []string
	sections       map[string]*sectionSummary
	currentSection string
	failureDetails []string
}

func newCargoTestCompactState() *cargoTestCompactState {
	return &cargoTestCompactState{
		sectionOrder:   make([]string, 0, 4),
		sections:       map[string]*sectionSummary{},
		currentSection: "unit",
		failureDetails: make([]string, 0, 32),
	}
}

func (s *cargoTestCompactState) consumeLine(line string) {
	norm := strings.TrimSpace(line)
	if norm == "" {
		return
	}
	if progressRe.MatchString(norm) {
		s.recognized++
		return
	}
	if sec, ok := testSection(norm); ok {
		s.recognized++
		s.currentSection = sec
		s.ensureSection(sec)
		return
	}
	if m := testResultRe.FindStringSubmatch(norm); len(m) > 0 {
		ignored := ""
		if len(m) > 4 {
			ignored = m[4]
		}
		s.recognized++
		sec := s.ensureSection(s.currentSection)
		sec.passed += atoiSafe(m[2])
		sec.failed += atoiSafe(m[3])
		sec.ignored += atoiSafe(ignored)
		return
	}
	s.consumeFailureLine(norm)
}

func (s *cargoTestCompactState) consumeFailureLine(norm string) {
	isFailure := failureMarkerRe.MatchString(norm) || docLineRefRe.MatchString(norm) || testFailPkgRe.MatchString(norm)
	if !isFailure && !assertSideRe.MatchString(norm) && !diagnosticRe.MatchString(norm) {
		return
	}
	s.recognized++
	s.failureDetails = append(s.failureDetails, normalizeCargoTestDetail(norm))
}

func (s *cargoTestCompactState) ensureSection(name string) *sectionSummary {
	if sec := s.sections[name]; sec != nil {
		return sec
	}
	sec := &sectionSummary{name: name}
	s.sections[name] = sec
	s.sectionOrder = append(s.sectionOrder, name)
	return sec
}

func (s *cargoTestCompactState) ensureCurrentSection() {
	if len(s.sections) > 0 {
		return
	}
	s.ensureSection(s.currentSection)
}

func (s *cargoTestCompactState) shouldDropPackageOnlyFailure() bool {
	if len(s.sections) != 1 || len(s.failureDetails) != 1 {
		return false
	}
	sec := s.sections[s.currentSection]
	if sec == nil || sec.passed != 0 || sec.failed != 0 {
		return false
	}
	return testFailPkgRe.MatchString(strings.TrimSpace(s.failureDetails[0]))
}

func (s *cargoTestCompactState) totals() (int, int, int) {
	totalPassed := 0
	totalFailed := 0
	totalIgnored := 0
	for _, sec := range s.sections {
		totalPassed += sec.passed
		totalFailed += sec.failed
		totalIgnored += sec.ignored
	}
	return totalPassed, totalFailed, totalIgnored
}

func (s *cargoTestCompactState) render() string {
	totalPassed, totalFailed, totalIgnored := s.totals()
	var b strings.Builder
	if totalFailed == 0 && len(s.failureDetails) == 0 {
		b.WriteString(fmt.Sprintf("cargo test: ok (%d passed, %d failed, %d ignored)\n", totalPassed, totalFailed, totalIgnored))
	} else {
		b.WriteString(fmt.Sprintf("cargo test: failed (%d passed, %d failed, %d ignored)\n", totalPassed, totalFailed, totalIgnored))
	}
	for _, secName := range s.sectionOrder {
		sec := s.sections[secName]
		if sec == nil {
			continue
		}
		b.WriteString(fmt.Sprintf("- %s: %d passed, %d failed, %d ignored\n", sec.name, sec.passed, sec.failed, sec.ignored))
	}
	orderedDetails := prioritizeDiagnostics(s.failureDetails)
	orderedDetails = collapseCargoTestFailureDetails(orderedDetails)
	if len(orderedDetails) == 0 {
		return b.String()
	}
	b.WriteString("failure details:\n")
	for i, line := range orderedDetails {
		if i >= maxDiagnostics {
			b.WriteString(fmt.Sprintf("... +%d more\n", len(orderedDetails)-maxDiagnostics))
			break
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func skipCargoFailureDetailLine(line string) bool {
	return line == "" ||
		line == "failures:" ||
		strings.HasPrefix(line, "---- ") ||
		testFailPkgRe.MatchString(line) ||
		strings.HasPrefix(line, "cargo test: failed (") ||
		line == "failure details:"
}

func parseCollapsedCargoFailure(testName string, lines []string, idx int) (string, int, []string) {
	advance := idx
	loc := ""
	msg := "FAILED"
	if idx+1 < len(lines) {
		if p := panicAtLine.FindStringSubmatch(strings.TrimSpace(lines[idx+1])); len(p) > 0 {
			loc = p[1]
			advance = idx + 1
			if advance+1 < len(lines) {
				nextLine := strings.TrimSpace(lines[advance+1])
				if strings.HasPrefix(nextLine, "assertion ") || strings.HasPrefix(nextLine, "panic:") {
					msg = nextLine
					advance++
				}
			}
		}
	}
	summary := fmt.Sprintf("- %s: %s", testName, msg)
	if loc != "" {
		summary = fmt.Sprintf("- %s (%s): %s", testName, loc, msg)
	}
	assertDetails := make([]string, 0, 2)
	for advance+1 < len(lines) {
		next := strings.TrimSpace(lines[advance+1])
		if !assertSideRe.MatchString(next) {
			break
		}
		assertDetails = append(assertDetails, next)
		advance++
	}
	return summary, advance, assertDetails
}

func testSection(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(trimmed, "Running unittests"):
		return "unit", true
	case strings.HasPrefix(trimmed, "Running tests/"):
		return "integration", true
	case strings.HasPrefix(trimmed, "Doc-tests"):
		return "doc", true
	default:
		return "", false
	}
}

func prioritizeDiagnostics(lines []string) []string {
	errors := make([]string, 0, len(lines))
	warnings := make([]string, 0, len(lines))
	other := make([]string, 0, len(lines))
	for _, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		switch {
		case strings.Contains(lower, "error") || strings.Contains(lower, "panic") || strings.Contains(lower, "failed"):
			errors = append(errors, line)
		case strings.Contains(lower, "warning"):
			warnings = append(warnings, line)
		default:
			other = append(other, line)
		}
	}
	out := make([]string, 0, len(lines))
	out = append(out, errors...)
	out = append(out, other...)
	out = append(out, warnings...)
	return out
}

func atoiSafe(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
