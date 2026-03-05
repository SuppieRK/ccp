package engine

import (
	"regexp"
	"strings"
)

var (
	uuidPattern    = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	hexPattern     = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	numericPattern = regexp.MustCompile(`\b[0-9]{10,}\b`)
)

func maskLine(s string) string {
	// Fixed order: UUID -> hex -> numeric. Skip regex work when patterns are impossible.
	if strings.Count(s, "-") >= 4 {
		s = uuidPattern.ReplaceAllString(s, "[UUID]")
	}
	if strings.Contains(s, "0x") || strings.Contains(s, "0X") {
		s = hexPattern.ReplaceAllString(s, "[ADDR]")
	}
	if strings.ContainsAny(s, "0123456789") {
		s = numericPattern.ReplaceAllString(s, "[NUM]")
	}
	return s
}
