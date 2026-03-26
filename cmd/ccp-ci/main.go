package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go-command-compression-proxy/internal/benchmark"
)

var (
	runBenchmarks          = benchmark.Run
	writeBenchmarkSummary  = benchmark.WriteSummary
	benchmarkFailureReport = benchmark.FailureSummary
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

type config struct {
	fixturesRoot   string
	artifactsDir   string
	tool           string
	previousReport string
}

func run(args []string, stderr io.Writer) int {
	cfg, code, err := parseConfig(args)
	if err != nil {
		return writeFailure(stderr, code, "%v", err)
	}
	resolvedRoot, err := resolveFixturesRoot(cfg.fixturesRoot, cfg.tool)
	if err != nil {
		return writeFailure(stderr, 1, "%v", err)
	}
	report, err := runBenchmarks(benchmark.RunOptions{
		FixturesRoot:   resolvedRoot,
		ArtifactsDir:   cfg.artifactsDir,
		ProxyBinary:    "ccp",
		Timeout:        2 * time.Minute,
		PreviousReport: cfg.previousReport,
	})
	if err != nil {
		return writeFailure(stderr, 1, "%v", err)
	}
	if err := writeBenchmarkSummary(report); err != nil {
		return writeFailure(stderr, 1, "%v", err)
	}
	if report.Failed {
		for _, line := range benchmarkFailureReport(report) {
			_, _ = fmt.Fprintln(stderr, line)
		}
		return 1
	}
	return 0
}

func parseConfig(args []string) (config, int, error) {
	var fixturesRoot string
	var artifactsDir string
	var tool string
	var previousReport string
	flags := flag.NewFlagSet("ccp-ci", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&fixturesRoot, "fixtures-root", filepath.Join("testdata", "benchmarks"), "fixture root")
	flags.StringVar(&artifactsDir, "artifacts-dir", filepath.Join(".artifacts", "benchmark"), "artifact output dir")
	flags.StringVar(&tool, "tool", "", "optional tool directory to run")
	flags.StringVar(&previousReport, "previous-report", "", "optional previous report.json for benchmark comparison")
	if err := flags.Parse(args); err != nil {
		return config{}, 2, err
	}
	return config{
		fixturesRoot:   fixturesRoot,
		artifactsDir:   artifactsDir,
		tool:           tool,
		previousReport: previousReport,
	}, 0, nil
}

func resolveFixturesRoot(fixturesRoot, tool string) (string, error) {
	trimmedTool := strings.TrimSpace(tool)
	if trimmedTool == "" {
		return fixturesRoot, nil
	}
	if !isSingleToolDirName(trimmedTool) {
		return "", fmt.Errorf("--tool must be a single tool directory name, got %q", tool)
	}
	return filepath.Join(fixturesRoot, trimmedTool), nil
}

func isSingleToolDirName(tool string) bool {
	if tool == "." || tool == ".." || filepath.Base(tool) != tool {
		return false
	}
	return !strings.ContainsAny(tool, `/\`)
}

func fatal(msg string) {
	os.Exit(writeFailure(os.Stderr, 1, "%s", msg))
}

func writeFailure(w io.Writer, code int, format string, args ...any) int {
	_, _ = fmt.Fprintf(w, format+"\n", args...)
	return code
}
