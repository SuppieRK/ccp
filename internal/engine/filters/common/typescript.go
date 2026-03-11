package common

import (
	"regexp"
	"sort"
	"strings"
)

var tscDiagRe = regexp.MustCompile(`^(.+?)\((\d+),(\d+)\):\s+(error|warning)\s+(TS\d+):\s+(.+)$`)

func TSCPrettyMode(args []string) (enabled bool, specified bool) {
	for i := 0; i < len(args); i++ {
		arg := LowerTrim(args[i])
		switch {
		case arg == "--pretty":
			if i+1 < len(args) {
				next := LowerTrim(args[i+1])
				if next == "false" {
					return false, true
				}
				if next == "true" {
					return true, true
				}
			}
			return true, true
		case strings.HasPrefix(arg, "--pretty="):
			value := strings.TrimSpace(arg[len("--pretty="):])
			return value != "false", true
		}
	}
	return false, false
}

func SummarizeTSCOutput(raw string) (string, bool) {
	type diag struct {
		file     string
		line     string
		col      string
		severity string
		code     string
		msg      string
	}
	diags := make([]diag, 0, 32)
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		m := tscDiagRe.FindStringSubmatch(trimmed)
		if len(m) != 7 {
			continue
		}

		diags = append(diags, diag{
			file:     m[1],
			line:     m[2],
			col:      m[3],
			severity: m[4],
			code:     m[5],
			msg:      m[6],
		})
	}

	if len(diags) == 0 {
		return "", false
	}

	sort.Slice(diags, func(i, j int) bool {
		if diags[i].file != diags[j].file {
			return diags[i].file < diags[j].file
		}
		if diags[i].line != diags[j].line {
			return diags[i].line < diags[j].line
		}
		if diags[i].col != diags[j].col {
			return diags[i].col < diags[j].col
		}
		return diags[i].code < diags[j].code
	})

	var b strings.Builder
	lastFile := ""
	for _, d := range diags {
		if d.file != lastFile {
			b.WriteString(d.file)
			b.WriteString(":\n")
			lastFile = d.file
		}
		b.WriteString("- ")
		b.WriteString(d.line)
		b.WriteString(":")
		b.WriteString(d.col)
		b.WriteString(" ")
		b.WriteString(d.severity)
		b.WriteString(" ")
		b.WriteString(d.code)
		b.WriteString(" ")
		b.WriteString(d.msg)
		b.WriteString("\n")
	}
	return b.String(), true
}

func CountTSCDiagnosticFiles(raw string) int {
	files := map[string]struct{}{}
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		m := tscDiagRe.FindStringSubmatch(trimmed)
		if len(m) != 7 {
			continue
		}
		files[m[1]] = struct{}{}
	}
	return len(files)
}
