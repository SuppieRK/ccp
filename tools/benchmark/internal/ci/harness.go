package ci

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type RunOptions struct {
	FixturesRoot     string
	ArtifactsDir     string
	ProxyBinary      string
	PreviousReport   string
	MaxArtifactBytes int
	Timeout          time.Duration
}

type CommandResult struct {
	Spec           string `json:"spec"`
	ExitCode       int    `json:"exit_code"`
	DurationMs     int64  `json:"duration_ms"`
	TokenCount     int    `json:"token_count"`
	Artifact       string `json:"artifact"`
	StdoutArtifact string `json:"stdout_artifact,omitempty"`
	StderrArtifact string `json:"stderr_artifact,omitempty"`
}

type ScenarioResult struct {
	Scenario             string        `json:"scenario"`
	Tool                 string        `json:"tool"`
	Status               string        `json:"status,omitempty"`
	TextOnly             bool          `json:"text_only,omitempty"`
	FixtureKey           string        `json:"fixture_key"`
	Pwd                  string        `json:"pwd"`
	RawInputHash         string        `json:"raw_input_hash"`
	Native               CommandResult `json:"native"`
	Proxy                CommandResult `json:"proxy"`
	Success              bool          `json:"success"`
	Required             bool          `json:"required"`
	ExitCodeMatch        bool          `json:"exit_code_match"`
	SafetyPassed         bool          `json:"safety_invariants_passed"`
	TokenCompactionRatio float64       `json:"token_compaction_ratio"`
	ProxyOverheadMs      int64         `json:"proxy_overhead_ms"`
	ProxyOverheadRatio   float64       `json:"proxy_overhead_ratio"`
	Warnings             []string      `json:"warnings,omitempty"`
}

type RunReport struct {
	Generated      time.Time        `json:"generated"`
	Tokenizer      string           `json:"tokenizer"`
	Results        []ScenarioResult `json:"results"`
	FailedRequired bool             `json:"failed_required"`
	FailedSafety   bool             `json:"failed_safety"`
}

type severityThresholds struct {
	YellowOverheadAbsMs int64
	YellowOverheadRel   float64
	YellowMinNativeMs   int64
}

const (
	benchmarkOutputFileName      = "output.txt"
	benchmarkInputStdoutFileName = "input-stdout.txt"
	benchmarkInputStderrFileName = "input-stderr.txt"
	benchmarkInputFileName       = "input.txt"
	structuredOutputMismatchWarn = "structured output mismatch"
)

func Run(opts RunOptions) (RunReport, error) {
	opts = withRunDefaults(opts)
	if err := os.MkdirAll(opts.ArtifactsDir, 0o755); err != nil {
		return RunReport{}, err
	}

	scenarios, err := discoverScenarios(opts.FixturesRoot)
	if err != nil {
		return RunReport{}, err
	}
	prevByScenario, err := loadPreviousResults(opts.PreviousReport)
	if err != nil {
		return RunReport{}, err
	}
	thresholds := loadSeverityThresholds()
	report := RunReport{
		Generated: time.Now().UTC(),
		Tokenizer: TokenizerID(),
		Results:   make([]ScenarioResult, 0, len(scenarios)),
	}
	for _, ds := range scenarios {
		res := runScenario(opts, ds)
		if prev, ok := prevByScenario[res.Scenario]; ok {
			maybeWarnCompactionDrop(&res, prev)
		}
		applyScenarioStatus(&res, thresholds)
		report.Results = append(report.Results, res)
		if ds.Spec.Required && !res.Success {
			report.FailedRequired = true
		}
		if !res.SafetyPassed {
			report.FailedSafety = true
		}
	}
	sort.Slice(report.Results, func(i, j int) bool { return report.Results[i].Scenario < report.Results[j].Scenario })
	if err := writeSummary(report); err != nil {
		return report, err
	}
	if err := writeReportJSON(opts, report); err != nil {
		return report, err
	}
	return report, nil
}

func withRunDefaults(opts RunOptions) RunOptions {
	if opts.FixturesRoot == "" {
		opts.FixturesRoot = filepath.Join("testdata", "tool-fixtures")
	}
	if opts.ArtifactsDir == "" {
		opts.ArtifactsDir = filepath.Join(".artifacts", "benchmark")
	}
	if opts.ProxyBinary == "" {
		opts.ProxyBinary = "ccp"
	}
	if opts.MaxArtifactBytes <= 0 {
		opts.MaxArtifactBytes = 5 * 1024 * 1024
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}
	return opts
}

func runScenario(opts RunOptions, ds DiscoveredScenario) (res ScenarioResult) {
	s := ds.Spec
	res = ScenarioResult{
		Scenario:   s.Name,
		Tool:       s.Tool,
		TextOnly:   s.TextOnly,
		FixtureKey: ds.FixtureKey,
		Native: CommandResult{
			Spec: strings.Join(s.Native, " "),
		},
		Required: s.Required,
	}
	proxyCmd := buildProxyCommand(s.Native, opts.ProxyBinary)
	res.Proxy.Spec = strings.Join(proxyCmd, " ")
	projectDir := filepath.Join(ds.Dir, s.Project)
	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		absProjectDir = projectDir
	}
	absFixturesRoot, rootErr := filepath.Abs(opts.FixturesRoot)
	if rootErr != nil {
		absFixturesRoot = opts.FixturesRoot
	}
	relProjectDir, relErr := filepath.Rel(absFixturesRoot, absProjectDir)
	if relErr != nil {
		relProjectDir = filepath.Join(s.Name, s.Project)
	}
	relProjectDir = filepath.ToSlash(relProjectDir)
	res.Pwd = relProjectDir

	if s.TextOnly {
		return runTextOnlyScenario(ds, res)
	}

	measured, firstPassOK := runScenarioFirstPass(absProjectDir, s, proxyCmd, opts.Timeout, &res.Warnings)
	if !firstPassOK {
		res.Success = false
		return
	}
	applyScenarioMeasurements(&res, measured)
	applyScenarioSafetyAndTokens(&res, s, measured.nativeOut, measured.proxyOut)

	artDir := scenarioArtifactDir(opts.ArtifactsDir, ds.Dir, res.FixtureKey, res.Scenario)
	_ = os.MkdirAll(artDir, 0o755)
	proxyPath, proxyTrunc, secondPassOK := runScenarioSecondPass(absProjectDir, s, proxyCmd, opts, artDir, &res)
	if !secondPassOK {
		res.Success = false
		return
	}
	res.Native.Artifact = ""
	res.Proxy.Artifact = proxyPath
	if proxyTrunc {
		res.Warnings = append(res.Warnings, "artifact truncation applied")
	}
	pruneEmptyScenarioArtifactDir(artDir)
	res.Success = res.SafetyPassed
	return
}

type scenarioMeasuredRun struct {
	nativeOut      string
	proxyOut       string
	nativeExit     int
	proxyExit      int
	nativeDuration time.Duration
	proxyDuration  time.Duration
}

func runScenarioFirstPass(absProjectDir string, s Scenario, proxyCmd []string, timeout time.Duration, warnings *[]string) (scenarioMeasuredRun, bool) {
	measured := scenarioMeasuredRun{}
	ok := runScenarioPass(absProjectDir, s.BeforeStart, s.AfterStop, timeout, "first_pass", warnings, func() bool {
		var err error
		// Best-effort warmup: ignore failures and continue measured run.
		_, _, _, _ = resolveOutput(absProjectDir, s.Native, timeout)
		// Isolate warmup side effects from measured execution while preserving
		// any shared resources that hooks intentionally keep (for example caches).
		if !runBetweenPassHooks(absProjectDir, s, timeout, "first_pass:warmup_reset", warnings) {
			return false
		}
		measured.nativeOut, measured.nativeExit, measured.nativeDuration, err = resolveOutput(absProjectDir, s.Native, timeout)
		if err != nil {
			*warnings = append(*warnings, "native execution failed: "+err.Error())
			return false
		}
		if !runBetweenPassHooks(absProjectDir, s, timeout, "first_pass", warnings) {
			return false
		}
		measured.proxyOut, measured.proxyExit, measured.proxyDuration, err = resolveOutput(absProjectDir, proxyCmd, timeout)
		if err != nil {
			*warnings = append(*warnings, "proxy execution failed: "+err.Error())
			return false
		}
		return true
	})
	return measured, ok
}

func runBetweenPassHooks(absProjectDir string, s Scenario, timeout time.Duration, phase string, warnings *[]string) bool {
	// Re-seed state so proxy run observes the same preconditions as native.
	midAfter := runHookCommands(absProjectDir, s.AfterStop, timeout, phase+":between_runs:after_stop", true)
	*warnings = append(*warnings, midAfter...)
	if hasHookFailure(midAfter) {
		return false
	}
	midBefore := runHookCommands(absProjectDir, s.BeforeStart, timeout, phase+":between_runs:before_start", false)
	*warnings = append(*warnings, midBefore...)
	return !hasHookFailure(midBefore)
}

func applyScenarioMeasurements(res *ScenarioResult, measured scenarioMeasuredRun) {
	res.Native.ExitCode = measured.nativeExit
	res.Proxy.ExitCode = measured.proxyExit
	res.Native.DurationMs = measured.nativeDuration.Milliseconds()
	res.Proxy.DurationMs = measured.proxyDuration.Milliseconds()
	res.ExitCodeMatch = measured.nativeExit == measured.proxyExit
	res.ProxyOverheadMs = res.Proxy.DurationMs - res.Native.DurationMs
	if res.ProxyOverheadMs < 0 {
		res.ProxyOverheadMs = 0
	}
	if res.Native.DurationMs > 0 {
		res.ProxyOverheadRatio = float64(res.Proxy.DurationMs) / float64(res.Native.DurationMs)
	}
}

func applyScenarioSafetyAndTokens(res *ScenarioResult, s Scenario, nativeOut, proxyOut string) {
	normNative := normalizeWithIgnores(nativeOut, s.IgnoreLines)
	normProxy := normalizeWithIgnores(proxyOut, s.IgnoreLines)
	safety := res.ExitCodeMatch && res.Native.ExitCode == s.ExpectExit && res.Proxy.ExitCode == s.ExpectExit
	for _, pat := range s.MustContain {
		if !strings.Contains(normProxy, pat) {
			safety = false
			res.Warnings = append(res.Warnings, "missing must_contain: "+pat)
		}
	}
	for _, pat := range s.MustNotContain {
		if strings.Contains(normProxy, pat) {
			safety = false
			res.Warnings = append(res.Warnings, "unexpected must_not_contain: "+pat)
		}
	}
	if s.StructuredOutput && normNative != normProxy {
		safety = false
		res.Warnings = append(res.Warnings, structuredOutputMismatchWarn)
	}
	res.SafetyPassed = safety
	res.Native.TokenCount = CountTokens(nativeOut)
	res.Proxy.TokenCount = CountTokens(proxyOut)
	res.RawInputHash = sha256Hex(nativeOut)
	if res.Proxy.TokenCount > 0 {
		res.TokenCompactionRatio = float64(res.Native.TokenCount) / float64(res.Proxy.TokenCount)
	}
	if res.TokenCompactionRatio == 0 {
		res.TokenCompactionRatio = 1
	}
}

func runScenarioSecondPass(absProjectDir string, s Scenario, proxyCmd []string, opts RunOptions, artDir string, res *ScenarioResult) (string, bool, bool) {
	proxyPath := ""
	proxyTrunc := false
	secondPassOK := runScenarioPass(absProjectDir, s.BeforeStart, s.AfterStop, opts.Timeout, "second_pass", &res.Warnings, func() bool {
		if supportsCaptureRaw(opts.ProxyBinary) {
			stdoutCap, stderrCap, warnings := captureProxyRawArtifacts(absProjectDir, s.Native, opts.ProxyBinary, artDir, opts.Timeout, opts.MaxArtifactBytes)
			res.Warnings = append(res.Warnings, warnings...)
			res.Proxy.StdoutArtifact = stdoutCap
			res.Proxy.StderrArtifact = stderrCap
			// Reset project state after capture-raw so the proxy output capture
			// reflects the same command preconditions as the measured pass.
			midAfter := runHookCommands(absProjectDir, s.AfterStop, opts.Timeout, "second_pass:after_capture:after_stop", true)
			res.Warnings = append(res.Warnings, midAfter...)
			if hasHookFailure(midAfter) {
				return false
			}
			midBefore := runHookCommands(absProjectDir, s.BeforeStart, opts.Timeout, "second_pass:before_output:before_start", false)
			res.Warnings = append(res.Warnings, midBefore...)
			if hasHookFailure(midBefore) {
				return false
			}
		}
		capturedProxyOut, _, _, err := resolveOutput(absProjectDir, proxyCmd, opts.Timeout)
		if err != nil {
			res.Warnings = append(res.Warnings, "proxy output capture failed: "+err.Error())
			return false
		}
		outputPath := filepath.Join(artDir, benchmarkOutputFileName)
		if capturedProxyOut == "" {
			// Keep artifact folders clean: do not create empty output files.
			_ = os.Remove(outputPath)
			proxyPath = ""
			proxyTrunc = false
			return true
		}
		proxyPath, proxyTrunc = writeArtifact(outputPath, capturedProxyOut, opts.MaxArtifactBytes)
		return true
	})
	return proxyPath, proxyTrunc, secondPassOK
}

func runTextOnlyScenario(ds DiscoveredScenario, res ScenarioResult) ScenarioResult {
	s := ds.Spec
	scenarioDir := filepath.Join(ds.Dir, s.Name)
	nativeOut, proxyOut, warnings := loadTextOnlyScenarioIO(scenarioDir)
	res.Warnings = append(res.Warnings, warnings...)

	res.Native.ExitCode = s.ExpectExit
	res.Proxy.ExitCode = s.ExpectExit
	res.Native.DurationMs = 0
	res.Proxy.DurationMs = 0
	res.ExitCodeMatch = true
	res.ProxyOverheadMs = 0
	res.ProxyOverheadRatio = 0

	normNative := normalizeWithIgnores(nativeOut, s.IgnoreLines)
	normProxy := normalizeWithIgnores(proxyOut, s.IgnoreLines)
	res.SafetyPassed = textOnlySafetyCheck(&res.Warnings, s, normNative, normProxy)

	res.Native.TokenCount = CountTokens(nativeOut)
	res.Proxy.TokenCount = CountTokens(proxyOut)
	res.RawInputHash = sha256Hex(nativeOut)
	if res.Proxy.TokenCount > 0 {
		res.TokenCompactionRatio = float64(res.Native.TokenCount) / float64(res.Proxy.TokenCount)
	}
	if res.TokenCompactionRatio == 0 {
		res.TokenCompactionRatio = 1
	}

	attachTextOnlyArtifacts(&res, scenarioDir)

	res.Success = res.SafetyPassed
	return res
}

func textOnlySafetyCheck(warnings *[]string, s Scenario, normNative, normProxy string) bool {
	safety := true
	for _, pat := range s.MustContain {
		if !strings.Contains(normProxy, pat) {
			safety = false
			*warnings = append(*warnings, "missing must_contain: "+pat)
		}
	}
	for _, pat := range s.MustNotContain {
		if strings.Contains(normProxy, pat) {
			safety = false
			*warnings = append(*warnings, "unexpected must_not_contain: "+pat)
		}
	}
	if s.StructuredOutput && normNative != normProxy {
		safety = false
		*warnings = append(*warnings, structuredOutputMismatchWarn)
	}
	return safety
}

func attachTextOnlyArtifacts(res *ScenarioResult, scenarioDir string) {
	if proxyPath := existingFile(filepath.Join(scenarioDir, benchmarkOutputFileName)); proxyPath != "" {
		res.Proxy.Artifact = proxyPath
	}
	if stdoutPath := existingFile(filepath.Join(scenarioDir, benchmarkInputStdoutFileName)); stdoutPath != "" {
		res.Proxy.StdoutArtifact = stdoutPath
	}
	if stderrPath := existingFile(filepath.Join(scenarioDir, benchmarkInputStderrFileName)); stderrPath != "" {
		res.Proxy.StderrArtifact = stderrPath
	}
}

func existingFile(path string) string {
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return path
	}
	return ""
}

func loadTextOnlyScenarioIO(scenarioDir string) (string, string, []string) {
	warnings := make([]string, 0, 2)
	outPath := filepath.Join(scenarioDir, benchmarkOutputFileName)
	outB, err := os.ReadFile(outPath)
	if err != nil {
		warnings = append(warnings, "text_only missing "+benchmarkOutputFileName+": "+err.Error())
	}
	proxyOut := string(outB)

	inputPath := filepath.Join(scenarioDir, benchmarkInputFileName)
	if inB, err := os.ReadFile(inputPath); err == nil {
		return string(inB), proxyOut, warnings
	}

	stdoutPath := filepath.Join(scenarioDir, benchmarkInputStdoutFileName)
	stderrPath := filepath.Join(scenarioDir, benchmarkInputStderrFileName)
	events := make([]sequencedLine, 0, 64)
	if b, err := os.ReadFile(stdoutPath); err == nil {
		events = append(events, parseSequencedLines(string(b), "stdout")...)
	}
	if b, err := os.ReadFile(stderrPath); err == nil {
		events = append(events, parseSequencedLines(string(b), "stderr")...)
	}
	if len(events) == 0 {
		warnings = append(warnings, "text_only missing input fixture (input.txt or input-stdout/input-stderr)")
		return "", proxyOut, warnings
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].seq != events[j].seq {
			return events[i].seq < events[j].seq
		}
		return events[i].stream < events[j].stream
	})
	var b strings.Builder
	for _, ev := range events {
		b.WriteString(ev.line)
		b.WriteByte('\n')
	}
	return b.String(), proxyOut, warnings
}

type sequencedLine struct {
	seq    int
	stream string
	line   string
}

func parseSequencedLines(raw, stream string) []sequencedLine {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	out := make([]sequencedLine, 0, len(lines))
	next := 0
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		seq := next
		txt := line
		if len(line) > 6 && line[5] == '|' {
			if n, err := strconv.Atoi(line[:5]); err == nil {
				seq = n
				txt = line[6:]
			}
		}
		out = append(out, sequencedLine{seq: seq, stream: stream, line: txt})
		next++
	}
	return out
}

func runScenarioPass(projectDir string, beforeStart [][]string, afterStop [][]string, timeout time.Duration, label string, warnings *[]string, run func() bool) bool {
	beforeWarnings := runHookCommands(projectDir, beforeStart, timeout, label+":before_start", false)
	*warnings = append(*warnings, beforeWarnings...)
	ok := false
	if !hasHookFailure(beforeWarnings) {
		ok = run()
	}
	afterWarnings := runHookCommands(projectDir, afterStop, timeout, label+":after_stop", true)
	*warnings = append(*warnings, afterWarnings...)
	if hasHookFailure(beforeWarnings) || hasHookFailure(afterWarnings) {
		return false
	}
	return ok
}

func runHookCommands(projectDir string, hooks [][]string, timeout time.Duration, phase string, reverse bool) []string {
	if len(hooks) == 0 {
		return nil
	}
	warnings := make([]string, 0)
	for idx := 0; idx < len(hooks); idx++ {
		i := idx
		if reverse {
			i = len(hooks) - 1 - idx
		}
		cmdArgs := hooks[i]
		spec := strings.Join(cmdArgs, " ")
		out, code, _, err := resolveOutput(projectDir, cmdArgs, timeout)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s hook failed (%s): %v", phase, spec, err))
			continue
		}
		if code != 0 {
			msg := fmt.Sprintf("%s hook failed (%s): exit code %d", phase, spec, code)
			out = strings.TrimSpace(out)
			if out != "" {
				msg += ": " + firstLine(out)
			}
			warnings = append(warnings, msg)
		}
	}
	return warnings
}

func hasHookFailure(warnings []string) bool {
	return len(warnings) > 0
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func buildProxyCommand(native []string, proxyBinary string) []string {
	if proxyBinary != "ccp" {
		out := make([]string, 0, len(native))
		out = append(out, proxyBinary)
		out = append(out, native[1:]...)
		return out
	}
	out := make([]string, 0, len(native)+1)
	out = append(out, "ccp")
	out = append(out, native...)
	return out
}

func buildProxyCaptureCommand(native []string, proxyBinary string, captureDir string) []string {
	if proxyBinary != "ccp" {
		out := make([]string, 0, len(native)+3)
		out = append(out, proxyBinary, "--capture-raw")
		if strings.TrimSpace(captureDir) != "" {
			out = append(out, "--capture-raw-dir", captureDir)
		}
		out = append(out, native[1:]...)
		return out
	}
	out := make([]string, 0, len(native)+4)
	out = append(out, "ccp", "--capture-raw")
	if strings.TrimSpace(captureDir) != "" {
		out = append(out, "--capture-raw-dir", captureDir)
	}
	out = append(out, native...)
	return out
}

func supportsCaptureRaw(proxyBinary string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(proxyBinary)))
	return base == "ccp" || base == "ccp.exe"
}

func captureProxyRawArtifacts(projectDir string, native []string, proxyBinary string, artDir string, timeout time.Duration, maxArtifactBytes int) (string, string, []string) {
	absArtDir, err := filepath.Abs(artDir)
	if err != nil {
		absArtDir = artDir
	}
	before := snapshotCaptureFiles(absArtDir)

	cmdArgs := buildProxyCaptureCommand(native, proxyBinary, absArtDir)
	_, _, _, err = resolveOutput(projectDir, cmdArgs, timeout)
	if err != nil {
		return "", "", []string{"capture-raw execution failed: " + err.Error()}
	}

	after := snapshotCaptureFiles(absArtDir)
	newStdout := newestCaptureFileDiff(after.stdout, before.set)
	newStderr := newestCaptureFileDiff(after.stderr, before.set)

	warnings := make([]string, 0, 2)

	var stdoutArtifact string
	var stderrArtifact string
	if newStdout != "" {
		if data, err := os.ReadFile(newStdout); err == nil {
			stdoutArtifact, _ = writeArtifact(filepath.Join(artDir, benchmarkInputStdoutFileName), string(data), maxArtifactBytes)
		} else {
			warnings = append(warnings, "capture-raw read stdout failed: "+err.Error())
		}
		_ = os.Remove(newStdout)
	}
	if newStderr != "" {
		if data, err := os.ReadFile(newStderr); err == nil {
			stderrArtifact, _ = writeArtifact(filepath.Join(artDir, benchmarkInputStderrFileName), string(data), maxArtifactBytes)
		} else {
			warnings = append(warnings, "capture-raw read stderr failed: "+err.Error())
		}
		_ = os.Remove(newStderr)
	}
	return stdoutArtifact, stderrArtifact, warnings
}

type captureSnapshot struct {
	stdout []string
	stderr []string
	set    map[string]struct{}
}

func snapshotCaptureFiles(dir string) captureSnapshot {
	stdout := listCaptureFiles(dir, "stdout")
	stderr := listCaptureFiles(dir, "stderr")
	set := make(map[string]struct{}, len(stdout)+len(stderr))
	for _, p := range stdout {
		set[p] = struct{}{}
	}
	for _, p := range stderr {
		set[p] = struct{}{}
	}
	return captureSnapshot{stdout: stdout, stderr: stderr, set: set}
}

func listCaptureFiles(dir, stream string) []string {
	files, _ := filepath.Glob(filepath.Join(dir, "ccp-capture-*-input-"+stream+".txt"))
	return files
}

func newestCaptureFileDiff(files []string, before map[string]struct{}) string {
	var newest string
	var newestMod time.Time
	for _, p := range files {
		if _, exists := before[p]; exists {
			continue
		}
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if newest == "" || info.ModTime().After(newestMod) {
			newest = p
			newestMod = info.ModTime()
		}
	}
	return newest
}

func resolveOutput(projectDir string, cmdArgs []string, timeout time.Duration) (string, int, time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if len(cmdArgs) == 0 {
		return "", 0, 0, fmt.Errorf("empty command")
	}
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = projectDir
	cmd.Env = benchmarkCommandEnv(projectDir)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	started := time.Now()
	err := cmd.Run()
	elapsed := time.Since(started)
	if err == nil {
		return out.String(), 0, elapsed, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.String(), ee.ExitCode(), elapsed, nil
	}
	return out.String(), 0, elapsed, err
}

func benchmarkCommandEnv(projectDir string) []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if strings.HasPrefix(kv, "GIT_CEILING_DIRECTORIES=") {
			continue
		}
		if strings.HasPrefix(kv, "GIT_DIR=") {
			continue
		}
		if strings.HasPrefix(kv, "GIT_WORK_TREE=") {
			continue
		}
		filtered = append(filtered, kv)
	}
	filtered = append(filtered, "GIT_CEILING_DIRECTORIES="+projectDir)
	// Force git commands to resolve repository context only in the scenario cwd.
	filtered = append(filtered, "GIT_DIR=.git")
	filtered = append(filtered, "GIT_WORK_TREE=.")
	return filtered
}

func scenarioArtifactDir(artifactsDir, scenarioDir, fixtureKey, scenario string) string {
	absArtifacts, aErr := filepath.Abs(artifactsDir)
	absScenarioDir, sErr := filepath.Abs(scenarioDir)
	if aErr == nil && sErr == nil && absArtifacts == absScenarioDir {
		return filepath.Join(artifactsDir, artifactScenarioDirName(scenario))
	}
	return filepath.Join(artifactsDir, sanitize(fixtureKey), artifactScenarioDirName(scenario))
}

func normalizeWithIgnores(in string, ignores []string) string {
	res, err := compileIgnoreRegexes(ignores)
	if err != nil {
		return strings.ReplaceAll(in, "\r\n", "\n")
	}
	return normalizeWithCompiledIgnores(in, res)
}

func compileIgnoreRegexes(ignores []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(ignores))
	for _, pattern := range ignores {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		out = append(out, re)
	}
	return out, nil
}

func normalizeWithCompiledIgnores(in string, ignores []*regexp.Regexp) string {
	in = strings.ReplaceAll(in, "\r\n", "\n")
	lines := strings.Split(in, "\n")
	keep := make([]string, 0, len(lines))
	for _, line := range lines {
		skip := false
		for _, re := range ignores {
			if re.MatchString(line) {
				skip = true
				break
			}
		}
		if !skip {
			keep = append(keep, line)
		}
	}
	return strings.Join(keep, "\n")
}

func writeArtifact(path, content string, max int) (string, bool) {
	truncated := false
	if len(content) > max {
		content = content[:max] + "\n[TRUNCATED]\n"
		truncated = true
	}
	_ = os.WriteFile(path, []byte(content), 0o644)
	return path, truncated
}

func pruneEmptyScenarioArtifactDir(artDir string) {
	if strings.TrimSpace(artDir) == "" {
		return
	}
	entries, err := os.ReadDir(artDir)
	if err != nil {
		return
	}
	if len(entries) == 0 {
		_ = os.Remove(artDir)
		return
	}
	keep := map[string]struct{}{
		benchmarkOutputFileName:      {},
		benchmarkInputStdoutFileName: {},
		benchmarkInputStderrFileName: {},
	}
	for _, e := range entries {
		if _, ok := keep[e.Name()]; ok {
			return
		}
	}
	_ = os.Remove(artDir)
}

func writeReportJSON(opts RunOptions, report RunReport) error {
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(opts.ArtifactsDir, "report.json"), b, 0o644)
}

func writeSummary(report RunReport) error {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Tokenizer: `%s`\n\n", report.Tokenizer))
	b.WriteString("| Status | Scenario | Required | Command | Native ms | Proxy ms | Overhead ms | Native tokens | Proxy tokens | Token savings % | Safety | Exit Match | Notes |\n")
	b.WriteString("|---|---|---|---|---:|---:|---:|---:|---:|---:|---|---|---|\n")
	rows := append([]ScenarioResult(nil), report.Results...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Scenario < rows[j].Scenario })
	for _, r := range rows {
		b.WriteString(summaryRow(r))
	}
	summaryPath := os.Getenv("GITHUB_STEP_SUMMARY")
	if summaryPath != "" {
		f, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		if _, err := f.WriteString(b.String()); err != nil {
			_ = f.Close()
			return err
		}
		return f.Close()
	}
	fmt.Print(b.String())
	return nil
}

func summaryRow(r ScenarioResult) string {
	cells := summaryRowCells(r)
	return fmt.Sprintf(
		"| %s | %s | %t | `%s` | %s | %s | %s | %d | %d | %.2f | %t | %t | %s |\n",
		cells.status,
		r.Scenario,
		r.Required,
		cells.command,
		cells.nativeMs,
		cells.proxyMs,
		cells.overheadMs,
		cells.nativeTokens,
		cells.proxyTokens,
		cells.tokenSavingsPct,
		cells.safety,
		cells.exitMatch,
		cells.notes,
	)
}

type summaryCells struct {
	status          string
	command         string
	nativeMs        string
	proxyMs         string
	overheadMs      string
	nativeTokens    int
	proxyTokens     int
	tokenSavingsPct float64
	safety          bool
	exitMatch       bool
	notes           string
}

func summaryRowCells(r ScenarioResult) summaryCells {
	nativeMs, proxyMs, overheadMs := summaryDurationCells(r)
	return summaryCells{
		status:          summaryStatusCell(r.Status),
		command:         sanitizeSummaryCell(r.Native.Spec),
		nativeMs:        nativeMs,
		proxyMs:         proxyMs,
		overheadMs:      overheadMs,
		nativeTokens:    r.Native.TokenCount,
		proxyTokens:     r.Proxy.TokenCount,
		tokenSavingsPct: normalizedTokenSavingsPct(r),
		safety:          r.SafetyPassed,
		exitMatch:       r.ExitCodeMatch,
		notes:           sanitizeSummaryCell(strings.Join(r.Warnings, "; ")),
	}
}

func normalizedTokenSavingsPct(r ScenarioResult) float64 {
	if r.Native.TokenCount <= 0 {
		return 0
	}
	pct := (1 - float64(r.Proxy.TokenCount)/float64(r.Native.TokenCount)) * 100
	if pct > -5 && pct < 5 {
		return 0
	}
	return pct
}

func loadSeverityThresholds() severityThresholds {
	return severityThresholds{
		YellowOverheadAbsMs: int64(envInt("BENCH_YELLOW_OVERHEAD_ABS_MS", 20)),
		YellowOverheadRel:   float64(envInt("BENCH_YELLOW_OVERHEAD_REL_PCT", 25)) / 100.0,
		YellowMinNativeMs:   int64(envInt("BENCH_YELLOW_MIN_NATIVE_MS", 50)),
	}
}

func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func applyScenarioStatus(res *ScenarioResult, thresholds severityThresholds) {
	if res == nil {
		return
	}
	if isRedScenario(*res) {
		res.Status = "red"
		return
	}
	if isYellowScenario(res, thresholds) {
		res.Status = "yellow"
		return
	}
	res.Status = "green"
}

func isRedScenario(res ScenarioResult) bool {
	if !res.SafetyPassed || !res.ExitCodeMatch || (res.Required && !res.Success) {
		return true
	}
	safetyWarnings := []string{
		"missing must_contain:",
		"unexpected must_not_contain:",
		"structured output mismatch",
		"hook failed",
	}
	for _, warning := range res.Warnings {
		for _, marker := range safetyWarnings {
			if strings.Contains(warning, marker) {
				return true
			}
		}
	}
	return false
}

func isYellowScenario(res *ScenarioResult, thresholds severityThresholds) bool {
	if res == nil {
		return false
	}
	for _, warning := range res.Warnings {
		if strings.Contains(warning, "token compaction ratio dropped") || strings.Contains(warning, "artifact truncation applied") {
			return true
		}
	}
	if res.TextOnly {
		return false
	}
	if res.Native.DurationMs < thresholds.YellowMinNativeMs {
		return false
	}
	if res.ProxyOverheadMs < thresholds.YellowOverheadAbsMs {
		return false
	}
	if res.ProxyOverheadRatio < (1 + thresholds.YellowOverheadRel) {
		return false
	}
	res.Warnings = append(res.Warnings, fmt.Sprintf("overhead regression +%dms (ratio %.2f)", res.ProxyOverheadMs, res.ProxyOverheadRatio))
	return true
}

func summaryStatusCell(status string) string {
	switch status {
	case "red":
		return "🔴"
	case "yellow":
		return "🟡"
	default:
		return "🟢"
	}
}

func summaryDurationCells(r ScenarioResult) (native string, proxy string, overhead string) {
	if r.TextOnly {
		return "N/A", "N/A", "N/A"
	}
	return fmt.Sprintf("%d", r.Native.DurationMs), fmt.Sprintf("%d", r.Proxy.DurationMs), fmt.Sprintf("%d", r.ProxyOverheadMs)
}

func sanitize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "/", "-")
	return s
}

func sanitizeSummaryCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "/")
	s = strings.TrimSpace(s)
	return s
}

func artifactScenarioDirName(scenario string) string {
	return sanitize(scenario)
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum[:])
}

func loadPreviousResults(path string) (map[string]ScenarioResult, error) {
	out := map[string]ScenarioResult{}
	if strings.TrimSpace(path) == "" {
		return out, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return nil, fmt.Errorf("read previous report: %w", err)
	}
	var report RunReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parse previous report: %w", err)
	}
	for _, r := range report.Results {
		out[r.Scenario] = r
	}
	return out, nil
}

func maybeWarnCompactionDrop(curr *ScenarioResult, prev ScenarioResult) {
	if curr == nil {
		return
	}
	if strings.TrimSpace(curr.RawInputHash) == "" || strings.TrimSpace(prev.RawInputHash) == "" {
		return
	}
	if curr.RawInputHash != prev.RawInputHash {
		return
	}
	if prev.TokenCompactionRatio <= 0 {
		return
	}
	if curr.TokenCompactionRatio < prev.TokenCompactionRatio*0.95 {
		curr.Warnings = append(curr.Warnings, fmt.Sprintf("token compaction ratio dropped from %.2f to %.2f", prev.TokenCompactionRatio, curr.TokenCompactionRatio))
	}
}
