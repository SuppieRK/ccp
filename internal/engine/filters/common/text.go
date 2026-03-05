package common

import "strings"

// TruncateWithSuffix truncates s to max bytes and appends suffix when truncated.
func TruncateWithSuffix(s string, max int, suffix string) string {
	if max < 0 {
		max = 0
	}
	if len(s) <= max {
		return s
	}
	return s[:max] + suffix
}

// LowerTrim canonicalizes string comparisons in arg and line scanners.
func LowerTrim(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
