package npxfilters

import "strings"

func stripNpxWrapperNoise(raw string) string {
	var out []string
	inInstallPrompt := false
	for _, rawLine := range strings.Split(raw, "\n") {
		line := strings.ReplaceAll(rawLine, "\r", "")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		switch lower := strings.ToLower(trimmed); {
		case strings.HasPrefix(lower, "need to install the following packages"):
			inInstallPrompt = true
			continue
		case inInstallPrompt && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")):
			continue
		case strings.HasPrefix(lower, "ok to proceed?"), strings.HasPrefix(lower, "npm warn exec"), strings.ContainsAny(trimmed, "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"):
			inInstallPrompt = false
			continue
		default:
			inInstallPrompt = false
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}
