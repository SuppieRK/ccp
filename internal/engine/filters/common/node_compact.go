package common

import (
	"fmt"
	"strings"
)

type nodeOutEntry struct {
	line       string
	warningKey string
}

func NodeCompactOutput(raw string) (string, bool) {
	if NodeLowConfidenceOutput(raw) {
		return raw, false
	}
	entries, warningCounts := collectNodeOutputEntries(raw)
	if len(entries) == 0 {
		return "", true
	}
	return renderNodeOutputEntries(entries, warningCounts), true
}

func collectNodeOutputEntries(raw string) ([]nodeOutEntry, map[string]int) {
	lines := strings.Split(raw, "\n")
	entries := make([]nodeOutEntry, 0, len(lines))
	warningCounts := map[string]int{}

	for _, rawLine := range lines {
		line := NodeCanonicalLine(rawLine)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if NodeIsProgressNoise(rawLine, trimmed) {
			continue
		}
		if NodeIsRuntimeWarning(trimmed) {
			key := NodeNormalizeWarningKey(trimmed)
			warningCounts[key]++
			if warningCounts[key] > 1 {
				continue
			}
			entries = append(entries, nodeOutEntry{line: line, warningKey: key})
			continue
		}
		entries = append(entries, nodeOutEntry{line: line})
	}
	return entries, warningCounts
}

func renderNodeOutputEntries(entries []nodeOutEntry, warningCounts map[string]int) string {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(e.line)
		b.WriteString("\n")
		if e.warningKey == "" {
			continue
		}
		if n := warningCounts[e.warningKey] - 1; n > 0 {
			b.WriteString(fmt.Sprintf("[+%d similar warnings]\n", n))
		}
	}
	return b.String()
}
