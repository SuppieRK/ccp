package benchmark

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go-command-compression-proxy/internal/metrics"
	"go-command-compression-proxy/internal/replay"
)

type RunOptions struct {
	FixturesRoot string
	ArtifactsDir string
	ProxyBinary  string
	Timeout      time.Duration
}

type RunReport struct {
	Generated time.Time    `json:"generated"`
	Results   []CaseResult `json:"results"`
	Failed    bool         `json:"failed"`
}

type CaseResult struct {
	Tool         string   `json:"tool"`
	Case         string   `json:"case"`
	Command      string   `json:"command"`
	NativeTokens int      `json:"native_tokens"`
	ProxyTokens  int      `json:"proxy_tokens"`
	Success      bool     `json:"success"`
	Warnings     []string `json:"warnings,omitempty"`
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
	report := RunReport{
		Generated: time.Now().UTC(),
		Results:   make([]CaseResult, 0, len(cases)),
	}
	for _, fixtureCase := range cases {
		result := runCase(opts, fixtureCase)
		report.Results = append(report.Results, result)
		if !result.Success {
			report.Failed = true
		}
	}
	sort.Slice(report.Results, func(i, j int) bool {
		if report.Results[i].Tool == report.Results[j].Tool {
			return report.Results[i].Case < report.Results[j].Case
		}
		return report.Results[i].Tool < report.Results[j].Tool
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
		opts.ProxyBinary = "ccp"
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
	result.NativeTokens = estimateTokens(replay.CombinedInput(events))

	if err := runVerifyFixture(opts.ProxyBinary, artifactDir, opts.Timeout); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("verify failed: %v", err))
		return result
	}

	verifyOutputPath := filepath.Join(artifactDir, replay.VerifyOutputFileName)
	verifyOutput, err := os.ReadFile(verifyOutputPath)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("read verify output: %v", err))
		return result
	}
	result.ProxyTokens = estimateTokens(string(verifyOutput))

	if warning := compareIfPresent(fixture.OutputPath, verifyOutputPath, "output"); warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	verifyDecisionsPath := filepath.Join(artifactDir, replay.VerifyDecisionsFileName)
	if warning := compareIfPresent(fixture.DecisionsPath, verifyDecisionsPath, "decisions"); warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	appendCaseMetrics(artifactDir, fixture.Command.Argv, result.NativeTokens, result.ProxyTokens)
	result.Success = len(result.Warnings) == 0
	return result
}

func copyFixtureInputs(fixture replay.Fixture, artifactDir string) error {
	for _, path := range []string{fixture.CommandPath, fixture.StdoutPath, fixture.StderrPath} {
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

func compareIfPresent(expectedPath, actualPath, label string) string {
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		return fmt.Sprintf("read %s fixture: %v", label, err)
	}
	actual, err := os.ReadFile(actualPath)
	if err != nil {
		return fmt.Sprintf("read verify %s: %v", label, err)
	}
	if string(expected) != string(actual) {
		return fmt.Sprintf("%s mismatch", label)
	}
	return ""
}

func appendCaseMetrics(artifactDir string, args []string, nativeTokens, proxyTokens int) {
	rawBytes := nativeTokens * 4
	keptBytes := proxyTokens * 4
	_ = os.MkdirAll(filepath.Join(artifactDir, ".ccp"), 0o755)
	_ = metrics.Append(filepath.Join(artifactDir, ".ccp", "gain.db"), metrics.RunMetric{
		Timestamp:  time.Now().UTC(),
		Command:    strings.Join(args, " "),
		Tool:       args[0],
		Dispatch:   args[0],
		RawBytes:   rawBytes,
		KeptBytes:  keptBytes,
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

func HashInput(events []replay.Event) string {
	sum := sha256.Sum256([]byte(replay.CombinedInput(events)))
	return fmt.Sprintf("%x", sum[:])
}
