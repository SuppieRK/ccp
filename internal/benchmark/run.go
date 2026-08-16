package benchmark

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/SuppieRK/cmdshape/internal/metrics"
	"github.com/SuppieRK/cmdshape/internal/replay"
)

type RunOptions struct {
	FixturesRoot   string
	ArtifactsDir   string
	ProxyBinary    string
	Timeout        time.Duration
	PreviousReport string
}

type RunReport struct {
	Generated time.Time    `json:"generated"`
	Results   []CaseResult `json:"results"`
	Failed    bool         `json:"failed"`
}

type CaseResult struct {
	Tool                 string   `json:"tool"`
	Case                 string   `json:"case"`
	Command              string   `json:"command"`
	InputHash            string   `json:"input_hash,omitempty"`
	NativeTokens         int      `json:"native_tokens"`
	ProxyTokens          int      `json:"proxy_tokens"`
	NativeBytes          int      `json:"native_bytes"`
	ProxyBytes           int      `json:"proxy_bytes"`
	TokenCompactionRatio float64  `json:"token_compaction_ratio"`
	Success              bool     `json:"success"`
	Warnings             []string `json:"warnings,omitempty"`
	Unasserted           []string `json:"unasserted,omitempty"`
}

type fixtureCase struct {
	tool string
	name string
	dir  string
}

var runVerifyFixture = func(proxyBinary, dir string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, proxyBinary, "verify", "--dir", dir)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func Run(opts RunOptions) (RunReport, error) {
	opts = withDefaults(opts)
	if err := os.MkdirAll(opts.ArtifactsDir, 0o755); err != nil {
		return RunReport{}, fmt.Errorf("create artifacts dir: %w", err)
	}
	cases, err := discoverFixtures(opts.FixturesRoot)
	if err != nil {
		return RunReport{}, err
	}
	prevByCase, err := loadPreviousResults(opts.PreviousReport)
	if err != nil {
		return RunReport{}, err
	}
	report := RunReport{
		Generated: time.Now().UTC(),
		Results:   make([]CaseResult, 0, len(cases)),
	}
	for _, fixtureCase := range cases {
		result := runCase(opts, fixtureCase)
		if prev, ok := prevByCase[comparisonKey(result.Tool, result.Case)]; ok {
			maybeWarnCompactionDrop(&result, prev)
		}
		report.Results = append(report.Results, result)
		if !result.Success {
			report.Failed = true
		}
	}
	slices.SortFunc(report.Results, func(left, right CaseResult) int {
		return cmp.Or(
			cmp.Compare(left.Tool, right.Tool),
			cmp.Compare(left.Case, right.Case),
		)
	})
	if err := writeReportJSON(opts.ArtifactsDir, report); err != nil {
		return report, err
	}
	return report, nil
}

func withDefaults(opts RunOptions) RunOptions {
	if strings.TrimSpace(opts.FixturesRoot) == "" {
		opts.FixturesRoot = filepath.Join("testdata", "benchmarks")
	}
	if strings.TrimSpace(opts.ArtifactsDir) == "" {
		opts.ArtifactsDir = filepath.Join(".artifacts", "benchmark")
	}
	if strings.TrimSpace(opts.ProxyBinary) == "" {
		opts.ProxyBinary = "cmdshape"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}
	return opts
}

func discoverFixtures(root string) ([]fixtureCase, error) {
	cases := make([]fixtureCase, 0, 32)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		commandPath := filepath.Join(path, replay.CommandFileName)
		info, err := os.Stat(commandPath)
		if err != nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) == 1 {
			cases = append(cases, fixtureCase{
				tool: filepath.Base(root),
				name: parts[0],
				dir:  path,
			})
			return filepath.SkipDir
		}
		if len(parts) < 2 {
			return fmt.Errorf("fixture %q must be under <tool>/<case>", path)
		}
		cases = append(cases, fixtureCase{
			tool: parts[0],
			name: parts[len(parts)-1],
			dir:  path,
		})
		return filepath.SkipDir
	})
	return cases, err
}

func runCase(opts RunOptions, item fixtureCase) CaseResult {
	fixture, err := replay.LoadFixture(item.dir)
	if err != nil {
		return CaseResult{Tool: item.tool, Case: item.name, Success: false, Warnings: []string{err.Error()}}
	}
	result := CaseResult{
		Tool:    item.tool,
		Case:    item.name,
		Command: strings.Join(fixture.Command.Argv, " "),
	}

	artifactDir := filepath.Join(opts.ArtifactsDir, item.tool, item.name)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return result
	}
	if err := copyFixtureInputs(fixture, artifactDir); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return result
	}

	events, err := replay.ReadEvents(fixture.StdoutPath, fixture.StderrPath)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return result
	}
	result.InputHash = fixtureInputHash(fixture.Command, events)
	nativeOutput := replay.CombinedInput(events)
	result.NativeBytes = len(nativeOutput)
	result.NativeTokens = estimateTokens(nativeOutput)

	if err := runVerifyFixture(opts.ProxyBinary, artifactDir, opts.Timeout); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("verify failed: %v", err))
		return result
	}

	verifyOutputPath := filepath.Join(artifactDir, replay.VerifyOutputFileName)
	if err := populateProxyMetrics(&result, verifyOutputPath); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return result
	}
	if result.ProxyBytes > result.NativeBytes {
		result.Warnings = append(result.Warnings, fmt.Sprintf(
			"output expansion: native=%d bytes proxy=%d bytes",
			result.NativeBytes,
			result.ProxyBytes,
		))
	}

	compareFixtureExpectations(&result, fixture, artifactDir, verifyOutputPath)
	if err := appendCaseMetrics(artifactDir, fixture.Command.Argv, result.NativeBytes, result.ProxyBytes); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("persist benchmark metrics: %v", err))
	}
	result.Success = len(result.Warnings) == 0
	return result
}

func populateProxyMetrics(result *CaseResult, verifyOutputPath string) error {
	verifyOutput, err := os.ReadFile(verifyOutputPath)
	if err != nil {
		return fmt.Errorf("read verify output: %w", err)
	}
	result.ProxyTokens = estimateTokens(string(verifyOutput))
	result.ProxyBytes = len(verifyOutput)
	result.TokenCompactionRatio = tokenCompactionRatio(result.NativeTokens, result.ProxyTokens)
	return nil
}

func compareFixtureExpectations(result *CaseResult, fixture replay.Fixture, artifactDir, verifyOutputPath string) {
	hasStdoutExpectation := regularFileExists(fixture.OutputStdoutPath)
	hasStderrExpectation := regularFileExists(fixture.OutputStderrPath)
	if hasStdoutExpectation || hasStderrExpectation {
		compareExpectation(result, fixture.OutputStdoutPath, filepath.Join(artifactDir, replay.VerifyStdoutFileName), "stdout")
		compareExpectation(result, fixture.OutputStderrPath, filepath.Join(artifactDir, replay.VerifyStderrFileName), "stderr")
	} else if regularFileExists(fixture.OutputPath) {
		compareExpectation(result, fixture.OutputPath, verifyOutputPath, "output")
	} else {
		result.Unasserted = append(result.Unasserted, "output expectation missing")
	}
	compareExpectation(result, fixture.DecisionsPath, filepath.Join(artifactDir, replay.VerifyDecisionsFileName), "decisions")
	compareExpectation(result, fixture.DispatchPath, filepath.Join(artifactDir, replay.VerifyDispatchFileName), "dispatch")
	if !fixture.Command.ExitCodeAsserted {
		result.Unasserted = append(result.Unasserted, "exit-code expectation missing")
	}
}

func compareExpectation(result *CaseResult, expectedPath, actualPath, label string) {
	if !regularFileExists(expectedPath) {
		result.Unasserted = append(result.Unasserted, label+" expectation missing")
		return
	}
	if warning := compareRequired(expectedPath, actualPath, label); warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
}

func copyFixtureInputs(fixture replay.Fixture, artifactDir string) error {
	for _, path := range []string{
		fixture.CommandPath,
		fixture.StdoutPath,
		fixture.StderrPath,
		fixture.OutputPath,
		fixture.OutputStdoutPath,
		fixture.OutputStderrPath,
		fixture.DecisionsPath,
		fixture.DispatchPath,
	} {
		if err := copyIfPresent(path, filepath.Join(artifactDir, filepath.Base(path))); err != nil {
			return err
		}
	}
	return nil
}

func copyIfPresent(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func compareRequired(expectedPath, actualPath, label string) string {
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		return fmt.Sprintf("read %s fixture: %v", label, err)
	}
	actual, err := os.ReadFile(actualPath)
	if err != nil {
		return fmt.Sprintf("read verify %s: %v", label, err)
	}
	if string(expected) != string(actual) {
		return label + " mismatch"
	}
	return ""
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func appendCaseMetrics(artifactDir string, args []string, nativeBytes, shapedBytes int) error {
	if err := os.MkdirAll(filepath.Join(artifactDir, ".cmdshape"), 0o755); err != nil {
		return err
	}
	return metrics.Append(metrics.ProjectPath(artifactDir), metrics.RunMetric{
		Timestamp:  time.Now().UTC(),
		Command:    strings.Join(args, " "),
		Tool:       args[0],
		Dispatch:   args[0],
		RawBytes:   nativeBytes,
		KeptBytes:  shapedBytes,
		ExitCode:   0,
		DurationMS: 0,
	})
}

func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + 3) / 4
}

func writeReportJSON(dir string, report RunReport) error {
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "report.json"), body, 0o644)
}

func FailureSummary(report RunReport) []string {
	lines := make([]string, 0, len(report.Results))
	for _, result := range report.Results {
		if result.Success {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s/%s: %s", result.Tool, result.Case, strings.Join(result.Warnings, "; ")))
	}
	return lines
}

func CorrectnessFailureSummary(report RunReport) []string {
	lines := make([]string, 0, len(report.Results))
	for _, result := range report.Results {
		failures := slices.DeleteFunc(slices.Clone(result.Warnings), func(warning string) bool {
			return strings.HasPrefix(warning, "net byte reduction dropped ")
		})
		failures = append(failures, result.Unasserted...)
		if len(failures) > 0 {
			lines = append(lines, fmt.Sprintf("%s/%s: %s", result.Tool, result.Case, strings.Join(failures, "; ")))
		}
	}
	return lines
}

func WriteSummary(report RunReport) error {
	var b strings.Builder
	rows := slices.Clone(report.Results)
	slices.SortFunc(rows, func(left, right CaseResult) int {
		return cmp.Or(
			cmp.Compare(left.Tool, right.Tool),
			cmp.Compare(left.Case, right.Case),
		)
	})
	for _, r := range rows {
		_, _ = fmt.Fprintf(
			&b,
			"| %s | %s | `%s` | %d | %d | %.2f | %s |\n",
			summaryStatusCell(r),
			sanitizeSummaryCell(r.Tool+"/"+r.Case),
			sanitizeSummaryCell(r.Command),
			r.NativeBytes,
			r.ProxyBytes,
			byteReductionPct(r),
			sanitizeSummaryCell(strings.Join(r.Warnings, "; ")),
		)
	}
	summaryPath := os.Getenv("GITHUB_STEP_SUMMARY")
	if summaryPath != "" {
		return appendSummaryToFile(summaryPath, b.String())
	}
	_, err := fmt.Print(summaryTableHeader() + b.String())
	return err
}

func appendSummaryToFile(path, rows string) error {
	prefix := ""
	if !summaryTableExists(path) {
		prefix = summaryTableHeader()
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(prefix + rows); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func summaryTableExists(path string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(body), summaryTableHeaderRow())
}

func summaryTableHeader() string {
	return summaryTableHeaderRow() + "\n" + "|---|---|---|---:|---:|---:|---|\n"
}

func summaryTableHeaderRow() string {
	return "| Status | Case | Command | Native bytes | Shaped bytes | Net byte reduction % | Notes |"
}

func summaryStatusCell(result CaseResult) string {
	if !result.Success {
		return "🔴"
	}
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "net byte reduction dropped") ||
			strings.Contains(warning, "token compaction ratio dropped") {
			return "🟡"
		}
	}
	return "🟢"
}

func byteReductionPct(result CaseResult) float64 {
	if result.NativeBytes <= 0 {
		return 0
	}
	return byteReductionRatio(result.NativeBytes, result.ProxyBytes) * 100
}

func sanitizeSummaryCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "/")
	return strings.TrimSpace(s)
}

func loadPreviousResults(path string) (map[string]CaseResult, error) {
	out := map[string]CaseResult{}
	if strings.TrimSpace(path) == "" {
		return out, nil
	}
	previous, err := readRunReport(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, fmt.Errorf("read previous report: %w", err)
	}
	for _, result := range previous.Results {
		out[comparisonKey(result.Tool, result.Case)] = result
	}
	return out, nil
}

func maybeWarnCompactionDrop(curr *CaseResult, prev CaseResult) {
	if strings.TrimSpace(curr.InputHash) == "" || strings.TrimSpace(prev.InputHash) == "" {
		return
	}
	if curr.InputHash != prev.InputHash {
		return
	}
	previousRatio, currentRatio := comparableReductionRatios(*curr, prev)
	if previousRatio <= 0 || currentRatio >= previousRatio*0.95 {
		return
	}
	curr.Warnings = append(curr.Warnings, fmt.Sprintf(
		"net byte reduction dropped from %.2f%% to %.2f%%",
		previousRatio*100,
		currentRatio*100,
	))
}

func comparableReductionRatios(curr, prev CaseResult) (float64, float64) {
	if prev.NativeBytes > 0 {
		return byteReductionRatio(prev.NativeBytes, prev.ProxyBytes),
			byteReductionRatio(curr.NativeBytes, curr.ProxyBytes)
	}
	if prev.TokenCompactionRatio <= 0 {
		return 0, 0
	}
	currentLegacyRatio := curr.TokenCompactionRatio
	if currentLegacyRatio <= 0 {
		currentLegacyRatio = tokenCompactionRatio(
			legacyEstimatedTokens(curr.NativeBytes),
			legacyEstimatedTokens(curr.ProxyBytes),
		)
	}
	return reductionFromCompactionRatio(prev.TokenCompactionRatio),
		reductionFromCompactionRatio(currentLegacyRatio)
}

func byteReductionRatio(nativeBytes, shapedBytes int) float64 {
	if nativeBytes <= 0 {
		return 0
	}
	return 1 - float64(shapedBytes)/float64(nativeBytes)
}

func reductionFromCompactionRatio(compactionRatio float64) float64 {
	if compactionRatio <= 0 {
		return 0
	}
	return 1 - 1/compactionRatio
}

func legacyEstimatedTokens(bytes int) int {
	if bytes <= 0 {
		return 0
	}
	return (bytes + 3) / 4
}

func tokenCompactionRatio(nativeTokens, proxyTokens int) float64 {
	if proxyTokens > 0 {
		return float64(nativeTokens) / float64(proxyTokens)
	}
	if nativeTokens > 0 {
		return float64(nativeTokens)
	}
	return 1
}

func readRunReport(path string) (RunReport, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return RunReport{}, err
	}
	var report RunReport
	if err := json.Unmarshal(body, &report); err != nil {
		return RunReport{}, fmt.Errorf("parse report json: %w", err)
	}
	return report, nil
}

func comparisonKey(tool, name string) string {
	return tool + "\x00" + name
}

func HashInput(events []replay.Event) string {
	sum := sha256.Sum256([]byte(replay.CombinedInput(events)))
	return hex.EncodeToString(sum[:])
}

func fixtureInputHash(command replay.CommandSpec, events []replay.Event) string {
	var b strings.Builder
	for _, arg := range command.Argv {
		b.WriteString(arg)
		b.WriteByte(0)
	}
	b.WriteString(strconv.Itoa(command.ExitCode))
	b.WriteByte(0)
	b.WriteString(replay.CombinedInput(events))
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}
