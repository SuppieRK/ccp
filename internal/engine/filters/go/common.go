package gofilters

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

const maxDiagnostics = 20

var (
	testOKRe      = regexp.MustCompile(`^ok\s+(\S+)\s+([0-9.]+s|\(cached\)|cached)`)
	testNoFilesRe = regexp.MustCompile(`^\?\s+(\S+)\s+\[no test files\]`)
	testFailPkgRe = regexp.MustCompile(`^FAIL\s+(\S+)`)
	buildIssueRe  = regexp.MustCompile(`\.go:\d+(:\d+)?:`)
	failMarkerRe  = regexp.MustCompile(`(^|\s)(--- FAIL:|FAIL\b|panic:|SIGSEGV)`)
	downloadingRe = regexp.MustCompile(`^go:\s+downloading\s+`)
	buildTraceRe  = regexp.MustCompile(`^[0-9]+\.[0-9]+s\s+#\s+`)
)

func isFailureMarker(line string) bool {
	return failMarkerRe.MatchString(strings.TrimSpace(line))
}

func shouldEmitFailureContext(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "--- FAIL:")
}

func joinTail(lines []string, n int) string {
	if len(lines) == 0 {
		return ""
	}
	if n <= 0 || n >= len(lines) {
		return strings.Join(lines, "")
	}
	return strings.Join(lines[len(lines)-n:], "")
}

func filteredFailureContext(lines []string, n int) string {
	if len(lines) == 0 {
		return ""
	}
	start := 0
	if n > 0 && len(lines) > n {
		start = len(lines) - n
	}
	var b strings.Builder
	for _, line := range lines[start:] {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r\n"))
		if trimmed == "" {
			continue
		}
		// Keep only pre-failure context. Existing failure markers are emitted
		// by their own immediate path and should not be replayed.
		if isFailureMarker(trimmed) {
			continue
		}
		b.WriteString(line)
	}
	return b.String()
}

func mapKeysSorted(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func compactBuildVet(raw, label string) (string, bool) {
	lines := filtercommon.NonEmptyLines(raw)
	if len(lines) == 0 {
		return label + ": ok\n", true
	}

	downloads := 0
	trace := 0
	diagnostics := make([]string, 0, 32)
	recognized := 0

	for _, line := range lines {
		kind := classifyBuildVetLine(line)
		if kind == buildVetReject {
			return "", false
		}
		if kind == buildVetIgnore {
			continue
		}

		recognized++
		switch kind {
		case buildVetIgnore, buildVetReject:
			// Guarded above; keep an exhaustive switch for enum-style linting.
		case buildVetTrace:
			trace++
		case buildVetDownload:
			downloads++
		case buildVetDiagnostic:
			diagnostics = append(diagnostics, line)
		}
	}

	if recognized == 0 {
		return "", false
	}
	return renderBuildVetSummary(label, downloads, trace, diagnostics), true
}

func renderBuildVetSummary(label string, downloads, trace int, diagnostics []string) string {
	var b strings.Builder
	diagCount := len(diagnostics)
	if diagCount == 0 {
		b.WriteString(label + ": ok\n")
	} else {
		_, _ = fmt.Fprintf(&b, "%s: %d diagnostics\n", label, diagCount)
	}
	if downloads > 0 {
		_, _ = fmt.Fprintf(&b, "[info] downloading %d dependencies...\n", downloads)
	}
	if trace > 0 {
		_, _ = fmt.Fprintf(&b, "[info] go build trace lines: %d\n", trace)
	}
	for i, line := range diagnostics {
		if i >= maxDiagnostics {
			_, _ = fmt.Fprintf(&b, "... +%d more\n", diagCount-maxDiagnostics)
			break
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

type buildVetLineKind int

const (
	buildVetIgnore buildVetLineKind = iota
	buildVetTrace
	buildVetDownload
	buildVetDiagnostic
	buildVetReject
)

func classifyBuildVetLine(line string) buildVetLineKind {
	if strings.ContainsRune(line, '\x00') && !isBuildTraceLine(line) {
		return buildVetReject
	}
	if isBuildTraceLine(line) {
		return buildVetTrace
	}
	if downloadingRe.MatchString(line) {
		return buildVetDownload
	}
	if buildIssueRe.MatchString(line) || isFailureMarker(line) {
		return buildVetDiagnostic
	}
	return buildVetIgnore
}

func isBuildTraceLine(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	switch {
	case strings.HasPrefix(t, "WORK="),
		strings.HasPrefix(t, "cd "),
		strings.HasPrefix(t, "mkdir "),
		strings.HasPrefix(t, "cat >"),
		t == "EOF",
		strings.HasPrefix(t, "packagefile "),
		strings.HasPrefix(t, "modinfo "),
		strings.HasPrefix(t, "GOROOT="):
		return true
	default:
		return buildTraceRe.MatchString(t)
	}
}
