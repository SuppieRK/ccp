package filters

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

const pipFormatOption = "--format"

// NewPIPFilter returns the built-in pip tool filter.
func NewPIPFilter() engine.ToolFilter { return pipFilter{} }

type pipFilter struct{}

func (pipFilter) Tool() string { return "pip" }

func (pipFilter) Aliases() []string {
	return []string{
		"pip3",
		"pip.exe", "./pip.exe",
		"pip3.exe", "./pip3.exe",
		"pip.cmd", "./pip.cmd",
		"pip3.cmd", "./pip3.cmd",
	}
}

func (pipFilter) Prepare(args []string) engine.PrepareResult {
	if len(args) == 0 {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}
	sub := filtercommon.LowerTrim(args[0])
	body := append([]string{}, args[1:]...)

	if hasPIPCompatibilitySensitiveFlag(body) {
		return engine.PrepareResult{
			NormalizedArgs:   args,
			ForcePassthrough: true,
			Ambiguous:        true,
			Reason:           "compatibility-sensitive pip flags",
		}
	}

	switch sub {
	case "list":
		if hasPIPExplicitJSONFormat(body) {
			return engine.PrepareResult{
				NormalizedArgs:   args,
				ForcePassthrough: true,
				Ambiguous:        true,
				Reason:           "structured output mode",
			}
		}
		if hasPIPExplicitNonJSONFormat(body) {
			return engine.PrepareResult{
				NormalizedArgs:   args,
				ForcePassthrough: true,
				Ambiguous:        true,
				Reason:           "explicit non-json --format for pip list",
			}
		}
		normalized := ensurePIPListJSON(body)
		return engine.PrepareResult{
			NormalizedArgs:        append([]string{"list"}, normalized...),
			DispatchKey:           "pip|mode=list",
			PreferredSubstitution: "uv",
			PreferredArgs:         append([]string{"pip", "list"}, normalized...),
			FallbackArgs:          append([]string{"list"}, normalized...),
		}
	case "outdated":
		if hasPIPExplicitJSONFormat(body) {
			return engine.PrepareResult{
				NormalizedArgs:   args,
				ForcePassthrough: true,
				Ambiguous:        true,
				Reason:           "structured output mode",
			}
		}
		if hasPIPExplicitNonJSONFormat(body) {
			return engine.PrepareResult{
				NormalizedArgs:   args,
				ForcePassthrough: true,
				Ambiguous:        true,
				Reason:           "explicit non-json --format for pip outdated",
			}
		}
		normalized := ensurePIPOutdatedJSON(body)
		return engine.PrepareResult{
			NormalizedArgs:        append([]string{"outdated"}, normalized...),
			DispatchKey:           "pip|mode=outdated",
			PreferredSubstitution: "uv",
			PreferredArgs:         append([]string{"pip", "list"}, normalized...),
			FallbackArgs:          append([]string{"outdated"}, normalized...),
		}
	case "install", "uninstall", "show":
		return engine.PrepareResult{
			NormalizedArgs:        append([]string{sub}, body...),
			ForcePassthrough:      true,
			PreferredSubstitution: "uv",
			PreferredArgs:         append([]string{"pip", sub}, body...),
			FallbackArgs:          append([]string{sub}, body...),
		}
	default:
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}
}

func (pipFilter) ContextKey(ev engine.Event) string {
	// list/outdated may emit warnings on stderr; keep context shared.
	return engine.SharedContextKey(ev)
}

func (pipFilter) MaskingHorizon() int { return 4096 }

func (pipFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	if d, ok := collectOnLineTickEOF(ev); ok {
		return d
	}
	switch ev.Type {
	case engine.EventExit:
		raw := mem.Joined()
		if strings.TrimSpace(raw) == "" {
			return engine.Decision{Action: engine.ActionIgnore}
		}
		mode := filtercommon.DispatchValue(ev.Dispatch, "mode")
		out, ok := compactPIPOutput(raw, mode)
		if !ok {
			return engine.Decision{Action: engine.ActionFlush, Output: raw}
		}
		if strings.TrimSpace(out) == "" {
			return engine.Decision{Action: engine.ActionFlush, Output: raw}
		}
		return engine.Decision{Action: engine.ActionFlush, Output: out}
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

func ensurePIPListJSON(args []string) []string {
	out := filtercommon.CopyArgs(args)
	if !filtercommon.HasOption(out, pipFormatOption) {
		out = append(out, "--format=json")
	}
	return out
}

func ensurePIPOutdatedJSON(args []string) []string {
	out := filtercommon.CopyArgs(args)
	// pip outdated equivalent for JSON is list --outdated --format=json
	if !filtercommon.HasExactFlag(out, "--outdated") {
		out = append(out, "--outdated")
	}
	if !filtercommon.HasOption(out, pipFormatOption) {
		out = append(out, "--format=json")
	}
	return out
}

func hasPIPExplicitNonJSONFormat(args []string) bool {
	v, ok := filtercommon.OptionValue(args, pipFormatOption)
	if !ok {
		return false
	}
	return !strings.EqualFold(v, "json")
}

func hasPIPExplicitJSONFormat(args []string) bool {
	return filtercommon.HasOptionValue(args, pipFormatOption, "json")
}

func hasPIPCompatibilitySensitiveFlag(args []string) bool {
	return filtercommon.HasAnyFlag(args, "--editable", "-e", "--user")
}

func compactPIPOutput(raw, mode string) (string, bool) {
	if lowConfidencePIPOutput(raw) {
		return raw, false
	}
	prefix, payload, suffix, ok := extractJSONArrayEnvelope(raw)
	if !ok {
		return raw, false
	}
	preservedPrefix := strings.TrimSpace(prefix)
	preservedSuffix := strings.TrimSpace(suffix)
	switch mode {
	case "list":
		out, ok := compactPIPList(payload)
		if !ok {
			return out, false
		}
		return combinePIPCompactedWithPreserved(out, preservedPrefix, preservedSuffix), true
	case "outdated":
		out, ok := compactPIPOutdated(payload)
		if !ok {
			return out, false
		}
		return combinePIPCompactedWithPreserved(out, preservedPrefix, preservedSuffix), true
	default:
		return raw, false
	}
}

func combinePIPCompactedWithPreserved(out, prefix, suffix string) string {
	parts := make([]string, 0, 3)
	if prefix != "" {
		parts = append(parts, prefix)
	}
	parts = append(parts, strings.TrimSuffix(out, "\n"))
	if suffix != "" {
		parts = append(parts, suffix)
	}
	return strings.Join(parts, "\n") + "\n"
}

func compactPIPList(payload string) (string, bool) {
	pkgs, ok := parsePIPPackageArray(payload)
	if !ok {
		return payload, false
	}
	if len(pkgs) == 0 {
		return "pip list: 0 packages\n", true
	}
	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].name) < strings.ToLower(pkgs[j].name)
	})
	lines := []string{fmt.Sprintf("pip list: %d packages", len(pkgs))}
	maxLines := min(8, len(pkgs))
	for i := 0; i < maxLines; i++ {
		lines = append(lines, fmt.Sprintf("  %s (%s)", pkgs[i].name, pkgs[i].version))
	}
	if len(pkgs) > maxLines {
		lines = append(lines, fmt.Sprintf("  ... +%d more", len(pkgs)-maxLines))
	}
	return strings.Join(lines, "\n") + "\n", true
}

func compactPIPOutdated(payload string) (string, bool) {
	pkgs, ok := parsePIPPackageArray(payload)
	if !ok {
		return payload, false
	}
	if len(pkgs) == 0 {
		return "pip outdated: 0 packages\n", true
	}
	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].name) < strings.ToLower(pkgs[j].name)
	})
	lines := []string{fmt.Sprintf("pip outdated: %d packages", len(pkgs))}
	maxLines := min(8, len(pkgs))
	for i := 0; i < maxLines; i++ {
		cur := pkgs[i].version
		latest := pkgs[i].latest
		if cur == "" || latest == "" {
			return payload, false
		}
		lines = append(lines, fmt.Sprintf("  %s %s -> %s", pkgs[i].name, cur, latest))
	}
	if len(pkgs) > maxLines {
		lines = append(lines, fmt.Sprintf("  ... +%d more", len(pkgs)-maxLines))
	}
	return strings.Join(lines, "\n") + "\n", true
}

type pipPackage struct {
	name    string
	version string
	latest  string
}

type pipPackageJSON struct {
	Name             string `json:"name"`
	Package          string `json:"package"`
	Project          string `json:"project"`
	Version          string `json:"version"`
	CurrentVersion   string `json:"current_version"`
	InstalledVersion string `json:"installed_version"`
	Current          string `json:"current"`
	LatestVersion    string `json:"latest_version"`
	Latest           string `json:"latest"`
	AvailableVersion string `json:"available_version"`
	NewVersion       string `json:"new_version"`
}

func parsePIPPackageArray(payload string) ([]pipPackage, bool) {
	var data []pipPackageJSON
	if json.Unmarshal([]byte(payload), &data) != nil {
		return nil, false
	}
	out := make([]pipPackage, 0, len(data))
	for _, item := range data {
		name := firstNonEmptyTrimmed(item.Name, item.Package, item.Project)
		ver := firstNonEmptyTrimmed(item.Version, item.CurrentVersion, item.InstalledVersion, item.Current)
		latest := firstNonEmptyTrimmed(item.LatestVersion, item.Latest, item.AvailableVersion, item.NewVersion)
		if name == "" {
			continue
		}
		out = append(out, pipPackage{name: name, version: ver, latest: latest})
	}
	return out, true
}

func extractJSONArrayEnvelope(raw string) (string, string, string, bool) {
	start := strings.Index(raw, "[")
	if start == -1 {
		return "", "", "", false
	}
	end, ok := findJSONArrayEnvelopeEnd(raw, start)
	if !ok {
		return "", "", "", false
	}
	return raw[:start], raw[start : end+1], raw[end+1:], true
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func findJSONArrayEnvelopeEnd(raw string, start int) (int, bool) {
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(raw); i++ {
		ch := raw[i]
		nextInString, nextEscaped, handled := advanceJSONArrayStringState(ch, inString, escaped)
		inString = nextInString
		escaped = nextEscaped
		if handled {
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		nextDepth, closed := advanceJSONArrayDepth(ch, depth)
		depth = nextDepth
		if closed {
			return i, true
		}
	}
	return 0, false
}

func advanceJSONArrayStringState(ch byte, inString, escaped bool) (bool, bool, bool) {
	if !inString {
		return inString, escaped, false
	}
	if escaped {
		return inString, false, true
	}
	if ch == '\\' {
		return inString, true, true
	}
	if ch == '"' {
		return false, escaped, true
	}
	return inString, escaped, true
}

func advanceJSONArrayDepth(ch byte, depth int) (int, bool) {
	switch ch {
	case '[':
		return depth + 1, false
	case ']':
		depth--
		return depth, depth == 0
	default:
		return depth, false
	}
}

func lowConfidencePIPOutput(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	lines := filtercommon.NonEmptyLines(raw)
	suspicious := 0
	for _, l := range lines {
		if !utf8.ValidString(l) {
			return true
		}
		if strings.ContainsRune(l, '\x00') {
			return true
		}
		if strings.HasPrefix(strings.TrimSpace(l), "{") && strings.Contains(l, "\"error\"") {
			suspicious++
		}
	}
	return suspicious > 4
}
