package filters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
)

const (
	pnpmOKMarker = "ok\n"
	pnpmJSONFlag = "--json"
)

// NewPNPMFilter returns the built-in pnpm tool filter.
func NewPNPMFilter() engine.ToolFilter { return pnpmFilter{} }

type pnpmFilter struct{}

func (pnpmFilter) Tool() string { return "pnpm" }

func (pnpmFilter) Aliases() []string {
	return []string{"pnpm.cmd", "./pnpm.cmd", "pnpm.exe", "./pnpm.exe"}
}

func (pnpmFilter) Prepare(args []string) engine.PrepareResult {
	if len(args) == 0 {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}
	sub := filtercommon.LowerTrim(args[0])
	body := append([]string{}, args[1:]...)

	switch sub {
	case "list":
		if filtercommon.HasExactFlag(body, pnpmJSONFlag) {
			return engine.PrepareResult{
				NormalizedArgs:   args,
				ForcePassthrough: true,
				Ambiguous:        true,
				Reason:           "structured output mode",
			}
		}
		normalized := ensurePnpmListStructuredArgs(body)
		return engine.PrepareResult{NormalizedArgs: append([]string{"list"}, normalized...), DispatchKey: "pnpm|mode=list"}
	case "outdated":
		if hasOutdatedFormatJSON(body) {
			return engine.PrepareResult{
				NormalizedArgs:   args,
				ForcePassthrough: true,
				Ambiguous:        true,
				Reason:           "structured output mode",
			}
		}
		normalized := ensurePnpmOutdatedStructuredArgs(body)
		return engine.PrepareResult{NormalizedArgs: append([]string{"outdated"}, normalized...), DispatchKey: "pnpm|mode=outdated"}
	case "install":
		pkgs := extractPnpmInstallPackages(body)
		for _, p := range pkgs {
			if !isSafePNPMPackageName(p) {
				return engine.PrepareResult{
					NormalizedArgs:   args,
					ForcePassthrough: true,
					Ambiguous:        true,
					Reason:           fmt.Sprintf("unsafe pnpm install package name: %s", p),
				}
			}
		}
		return engine.PrepareResult{NormalizedArgs: append([]string{"install"}, body...), DispatchKey: "pnpm|mode=install"}
	default:
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}
}

func (pnpmFilter) ContextKey(ev engine.Event) string {
	// list/outdated/install may emit mixed stdout/stderr diagnostics.
	return engine.SharedContextKey(ev)
}

func (pnpmFilter) MaskingHorizon() int { return 4096 }

func (pnpmFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	cfg := pnpmDispatch{mode: filtercommon.DispatchValue(ev.Dispatch, "mode")}
	switch ev.Type {
	case engine.EventLine, engine.EventTick:
		return engine.Decision{Action: engine.ActionCollect}
	case engine.EventEOF:
		return engine.Decision{Action: engine.ActionCollect}
	case engine.EventExit:
		return processPNPMExit(mem.Joined(), cfg, ev.ExitCode)
	default:
		return engine.Decision{Action: engine.ActionCollect}
	}
}

type pnpmDispatch struct {
	mode string
}

func ensurePnpmListStructuredArgs(args []string) []string {
	normalized := filtercommon.CopyArgs(args)
	if !filtercommon.HasExactFlag(normalized, pnpmJSONFlag) {
		normalized = append(normalized, pnpmJSONFlag)
	}
	if !hasAnyDepthFlag(normalized) {
		normalized = append(normalized, "--depth=0")
	}
	return normalized
}

func ensurePnpmOutdatedStructuredArgs(args []string) []string {
	normalized := filtercommon.CopyArgs(args)
	if !hasOutdatedFormatJSON(normalized) {
		normalized = append(normalized, "--format", "json")
	}
	return normalized
}

func hasAnyDepthFlag(args []string) bool {
	for i := 0; i < len(args); i++ {
		a := strings.TrimSpace(args[i])
		if strings.EqualFold(a, "--depth") && i+1 < len(args) {
			return true
		}
		if strings.HasPrefix(strings.ToLower(a), "--depth=") {
			return true
		}
	}
	return false
}

func hasOutdatedFormatJSON(args []string) bool {
	return filtercommon.HasOptionValue(args, "--format", "json")
}

func extractPnpmInstallPackages(args []string) []string {
	pkgs := make([]string, 0, len(args))
	i := 0
	for i < len(args) {
		a := strings.TrimSpace(args[i])
		if a == "" {
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			i = advancePnpmInstallFlagIndex(args, i, a)
			continue
		}
		pkgs = append(pkgs, a)
		i++
	}
	return pkgs
}

var pnpmSafePackageRe = regexp.MustCompile(`^(@[a-z0-9-~][a-z0-9-._~]*/)?[a-z0-9-~][a-z0-9-._~]*(?:@[a-z0-9-._~]+)?$`)

func isSafePNPMPackageName(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || len(trimmed) > 214 {
		return false
	}
	if strings.Contains(trimmed, "..") || strings.Contains(trimmed, "../") || strings.Contains(trimmed, "..\\") {
		return false
	}
	if strings.ContainsAny(trimmed, ";&|$><`\\\"'") {
		return false
	}
	return pnpmSafePackageRe.MatchString(trimmed)
}

func compactPNPMOutput(raw string, cfg pnpmDispatch, exitCode int) (string, bool) {
	if lowConfidencePNPMOutput(raw) {
		return raw, false
	}
	switch cfg.mode {
	case "list":
		return compactPNPMList(raw)
	case "outdated":
		return compactPNPMOutdated(raw, exitCode)
	case "install":
		return compactPNPMInstall(raw, exitCode), true
	default:
		return raw, false
	}
}

func compactPNPMList(raw string) (string, bool) {
	total, pkgs, ok := parsePNPMListStructured(raw)
	if ok {
		if total == 0 {
			return "dependencies: 0\n", true
		}
		lines := []string{fmt.Sprintf("dependencies: %d", total)}
		n := min(8, len(pkgs))
		for i := 0; i < n; i++ {
			lines = append(lines, "  "+pkgs[i])
		}
		if len(pkgs) > n {
			lines = append(lines, fmt.Sprintf("  ... +%d more", len(pkgs)-n))
		}
		return strings.Join(lines, "\n") + "\n", true
	}

	deps := parsePNPMListDegraded(raw)
	if len(deps) == 0 {
		return truncatePNPM(raw), false
	}
	lines := []string{fmt.Sprintf("dependencies: %d", len(deps))}
	for i := 0; i < min(8, len(deps)); i++ {
		lines = append(lines, "  "+deps[i])
	}
	if len(deps) > 8 {
		lines = append(lines, fmt.Sprintf("  ... +%d more", len(deps)-8))
	}
	return strings.Join(lines, "\n") + "\n", true
}

func parsePNPMListStructured(raw string) (int, []string, bool) {
	payload, err := decodePNPMJSON(raw)
	if err != nil {
		return 0, nil, false
	}
	pkgs := map[string]struct{}{}
	collectPnpmPackages(payload, pkgs)
	sorted := mapKeysSorted(pkgs)
	return len(sorted), sorted, true
}

func collectPnpmPackages(v any, seen map[string]struct{}) {
	switch t := v.(type) {
	case map[string]any:
		name, hasName := stringAt(t, "name")
		ver, hasVer := stringAt(t, "version")
		if hasName && hasVer {
			seen[name+"@"+ver] = struct{}{}
		}
		for _, k := range []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"} {
			if child, ok := t[k]; ok {
				collectPnpmDependencyMap(child, seen)
				collectPnpmPackages(child, seen)
			}
		}
		for _, child := range t {
			collectPnpmPackages(child, seen)
		}
	case []any:
		for _, item := range t {
			collectPnpmPackages(item, seen)
		}
	}
}

func collectPnpmDependencyMap(v any, seen map[string]struct{}) {
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	for depName, depVal := range m {
		depObj, ok := depVal.(map[string]any)
		if !ok {
			continue
		}
		if ver, hasVer := stringAt(depObj, "version"); hasVer && strings.TrimSpace(depName) != "" {
			seen[depName+"@"+ver] = struct{}{}
		}
	}
}

func parsePNPMListDegraded(raw string) []string {
	seen := map[string]struct{}{}
	for _, line := range filtercommon.NonEmptyLines(raw) {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") || strings.Contains(trimmed, "{") || strings.Contains(trimmed, "}") {
			continue
		}
		parts := strings.Fields(trimmed)
		if len(parts) == 0 {
			continue
		}
		candidate := parts[0]
		if strings.Count(candidate, "@") < 1 {
			continue
		}
		at := strings.LastIndex(candidate, "@")
		if at <= 0 || at >= len(candidate)-1 {
			continue
		}
		seen[candidate] = struct{}{}
	}
	return mapKeysSorted(seen)
}

func compactPNPMOutdated(raw string, exitCode int) (string, bool) {
	entries, out, ok, done := resolvePNPMOutdatedEntries(raw, exitCode)
	if done {
		return out, ok
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	outdated := 0
	lines := make([]string, 0, len(entries)+1)
	for _, e := range entries {
		if e.current != "" && e.latest != "" && e.current != e.latest {
			outdated++
		}
		lines = append(lines, formatPNPMOutdatedLine(e))
	}
	head := fmt.Sprintf("outdated: %d/%d", outdated, len(entries))
	if len(lines) > 8 {
		lines = append(lines[:8], fmt.Sprintf("... +%d more", len(lines)-8))
	}
	return head + "\n" + strings.Join(lines, "\n") + "\n", true
}

func resolvePNPMOutdatedEntries(raw string, exitCode int) ([]pnpmOutdatedEntry, string, bool, bool) {
	entries, ok := parsePNPMOutdatedStructured(raw)
	if ok {
		if len(entries) == 0 {
			if exitCode == 0 {
				return nil, "", true, true
			}
			return nil, raw, false, true
		}
		return entries, "", true, false
	}
	entries = parsePNPMOutdatedDegraded(raw)
	if len(entries) == 0 {
		if exitCode == 0 {
			return nil, "", true, true
		}
		return nil, truncatePNPM(raw), false, true
	}
	return entries, "", true, false
}

func formatPNPMOutdatedLine(e pnpmOutdatedEntry) string {
	target := e.wanted
	if strings.TrimSpace(target) == "" {
		if strings.TrimSpace(e.latest) == "" {
			target = "?"
		} else {
			target = e.latest
		}
	}
	current := e.current
	if strings.TrimSpace(current) == "" {
		current = "?"
	}
	return fmt.Sprintf("%s  %s -> %s", e.name, current, target)
}

type pnpmOutdatedEntry struct {
	name    string
	current string
	wanted  string
	latest  string
}

func parsePNPMOutdatedStructured(raw string) ([]pnpmOutdatedEntry, bool) {
	payload, err := decodePNPMJSON(raw)
	if err != nil {
		return nil, false
	}
	entries := make([]pnpmOutdatedEntry, 0)
	collectPnpmOutdated(payload, &entries)
	if len(entries) == 0 {
		return nil, true
	}
	uniq := map[string]pnpmOutdatedEntry{}
	for _, e := range entries {
		if e.name == "" {
			continue
		}
		uniq[e.name] = e
	}
	out := make([]pnpmOutdatedEntry, 0, len(uniq))
	for _, e := range uniq {
		out = append(out, e)
	}
	return out, true
}

func collectPnpmOutdated(v any, out *[]pnpmOutdatedEntry) {
	switch t := v.(type) {
	case []any:
		for _, item := range t {
			collectPnpmOutdated(item, out)
		}
	case map[string]any:
		name := pnpmOutdatedEntryName(t)
		current, hasCurrent := stringAt(t, "current")
		latest, hasLatest := stringAt(t, "latest")
		wanted, _ := stringAt(t, "wanted")
		if name != "" && (hasCurrent || hasLatest) {
			*out = append(*out, pnpmOutdatedEntry{name: name, current: current, wanted: wanted, latest: latest})
		}
		if name == "" {
			collectPnpmOutdatedNestedMapEntries(t, out)
		}
		for _, child := range t {
			collectPnpmOutdated(child, out)
		}
	}
}

func parsePNPMOutdatedDegraded(raw string) []pnpmOutdatedEntry {
	entries := make([]pnpmOutdatedEntry, 0)
	seen := map[string]struct{}{}
	for _, line := range filtercommon.NonEmptyLines(raw) {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), "legend:") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(trimmed), "package") || strings.Contains(trimmed, "----") {
			continue
		}
		parts := strings.Fields(trimmed)
		if len(parts) < 4 {
			continue
		}
		name := parts[0]
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		entries = append(entries, pnpmOutdatedEntry{name: name, current: parts[1], wanted: parts[2], latest: parts[3]})
	}
	return entries
}

func compactPNPMInstall(raw string, exitCode int) string {
	lines := filtercommon.NonEmptyLines(raw)
	out := make([]string, 0, len(lines))
	seen := map[string]struct{}{}

	for _, line := range lines {
		canonical := strings.TrimSpace(strings.ReplaceAll(line, "\r", ""))
		if isPNPMInstallProgress(canonical) {
			continue
		}
		appendPNPMInstallLine(canonical, seen, &out)
	}

	if len(out) == 0 {
		if exitCode == 0 {
			return pnpmOKMarker
		}
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

func processPNPMExit(raw string, cfg pnpmDispatch, exitCode int) engine.Decision {
	if strings.TrimSpace(raw) == "" {
		return pnpmExitDecisionForEmptyRaw(cfg.mode, exitCode)
	}
	out, ok := compactPNPMOutput(raw, cfg, exitCode)
	if !ok {
		return pnpmFlushFallbackDecision(raw, out)
	}
	if strings.TrimSpace(out) == "" {
		if exitCode != 0 {
			return engine.Decision{Action: engine.ActionFlush, Output: raw}
		}
		return pnpmSuccessMarkerDecision(cfg.mode)
	}
	return engine.Decision{Action: engine.ActionFlush, Output: out}
}

func pnpmExitDecisionForEmptyRaw(mode string, exitCode int) engine.Decision {
	if exitCode != 0 {
		return engine.Decision{Action: engine.ActionIgnore}
	}
	return pnpmSuccessMarkerDecision(mode)
}

func pnpmSuccessMarkerDecision(mode string) engine.Decision {
	switch mode {
	case "outdated":
		return engine.Decision{Action: engine.ActionFlush, Output: "All packages up-to-date\n"}
	case "install":
		return engine.Decision{Action: engine.ActionFlush, Output: pnpmOKMarker}
	default:
		return engine.Decision{Action: engine.ActionIgnore}
	}
}

func pnpmFlushFallbackDecision(raw, out string) engine.Decision {
	if strings.TrimSpace(out) != "" {
		return engine.Decision{Action: engine.ActionFlush, Output: out}
	}
	return engine.Decision{Action: engine.ActionFlush, Output: raw}
}

func advancePnpmInstallFlagIndex(args []string, i int, arg string) int {
	if pnpmInstallFlagConsumesNext(filtercommon.LowerTrim(arg)) && i+1 < len(args) {
		return i + 2
	}
	if strings.EqualFold(arg, "--depth") && i+1 < len(args) {
		return i + 2
	}
	return i + 1
}

func pnpmInstallFlagConsumesNext(arg string) bool {
	switch arg {
	case "--save-peer", "--save-prod", "--save-dev", "--save-optional",
		"--save-exact", "--workspace-root", "--filter", "-c", "--config":
		return true
	default:
		return false
	}
}

func pnpmOutdatedEntryName(m map[string]any) string {
	for _, key := range []string{"name", "package", "packageName"} {
		if name, _ := stringAt(m, key); name != "" {
			return name
		}
	}
	return ""
}

func collectPnpmOutdatedNestedMapEntries(m map[string]any, out *[]pnpmOutdatedEntry) {
	// map object: {"pkg": {"current":..., "latest":...}}
	for k, child := range m {
		entry, ok := parsePnpmOutdatedNestedEntry(k, child)
		if !ok {
			continue
		}
		*out = append(*out, entry)
	}
}

func parsePnpmOutdatedNestedEntry(name string, child any) (pnpmOutdatedEntry, bool) {
	m, ok := child.(map[string]any)
	if !ok {
		return pnpmOutdatedEntry{}, false
	}
	cur, hasCur := stringAt(m, "current")
	lat, hasLat := stringAt(m, "latest")
	if !hasCur && !hasLat {
		return pnpmOutdatedEntry{}, false
	}
	wan, _ := stringAt(m, "wanted")
	return pnpmOutdatedEntry{name: name, current: cur, wanted: wan, latest: lat}, true
}

func appendPNPMInstallLine(canonical string, seen map[string]struct{}, out *[]string) {
	if !isPNPMInstallFailure(canonical) && !isPNPMInstallSummary(canonical) {
		return
	}
	k := strings.ToLower(canonical)
	if _, ok := seen[k]; ok {
		return
	}
	seen[k] = struct{}{}
	*out = append(*out, canonical)
}

var pnpmProgressRe = regexp.MustCompile(`^(Progress:\s*\d+|\s*[+\-]\d+)`)

func isPNPMInstallProgress(line string) bool {
	lower := filtercommon.LowerTrim(line)
	if strings.Contains(line, "\x1b[") {
		return true
	}
	if strings.Contains(lower, "resolved") && strings.Contains(lower, "downloaded") {
		return true
	}
	if strings.Contains(lower, "packages: +") || strings.Contains(lower, "packages: -") {
		return true
	}
	return pnpmProgressRe.MatchString(line)
}

func isPNPMInstallFailure(line string) bool {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "err_pnpm") {
		return true
	}
	if strings.Contains(lower, " error") || strings.HasPrefix(lower, "error") {
		return true
	}
	return strings.Contains(lower, "failed")
}

func isPNPMInstallSummary(line string) bool {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "dependencies") {
		return true
	}
	if strings.Contains(lower, "devdependencies") || strings.Contains(lower, "optionaldependencies") {
		return true
	}
	if strings.Contains(lower, "done in ") {
		return true
	}
	if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
		return true
	}
	return false
}

func lowConfidencePNPMOutput(raw string) bool {
	if strings.ContainsRune(raw, '\x00') {
		return true
	}
	total := 0
	control := 0
	for _, r := range raw {
		if !utf8.ValidRune(r) {
			return true
		}
		total++
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			control++
		}
	}
	if total == 0 {
		return false
	}
	return control*100/total > 20
}

func truncatePNPM(raw string) string {
	const maxOutputChars = 1200
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	out := filtercommon.TruncateWithSuffix(trimmed, maxOutputChars, "\n... (truncated)\n")
	if len(trimmed) <= maxOutputChars {
		return out + "\n"
	}
	return out
}

func stringAt(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	case json.Number:
		return t.String(), true
	default:
		b, err := json.Marshal(t)
		if err != nil || bytes.Equal(b, []byte("null")) {
			return "", false
		}
		return string(b), true
	}
}

func mapKeysSorted(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func decodePNPMJSON(raw string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var payload any
	if err := dec.Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}
