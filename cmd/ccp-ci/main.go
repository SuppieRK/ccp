package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go-command-compression-proxy/internal/benchmark"
)

func main() {
	var fixturesRoot string
	var artifactsDir string
	var tool string
	var previousReport string
	flag.StringVar(&fixturesRoot, "fixtures-root", filepath.Join("testdata", "benchmarks"), "fixture root")
	flag.StringVar(&artifactsDir, "artifacts-dir", filepath.Join(".artifacts", "benchmark"), "artifact output dir")
	flag.StringVar(&tool, "tool", "", "optional tool directory to run")
	flag.StringVar(&previousReport, "previous-report", "", "optional previous report.json for benchmark comparison")
	flag.Parse()

	resolvedRoot, err := resolveFixturesRoot(fixturesRoot, tool)
	if err != nil {
		fatal(err.Error())
	}
	report, err := benchmark.Run(benchmark.RunOptions{
		FixturesRoot:   resolvedRoot,
		ArtifactsDir:   artifactsDir,
		ProxyBinary:    "ccp",
		Timeout:        2 * time.Minute,
		PreviousReport: previousReport,
	})
	if err != nil {
		fatal(err.Error())
	}
	if err := benchmark.WriteSummary(report); err != nil {
		fatal(err.Error())
	}
	if report.Failed {
		for _, line := range benchmark.FailureSummary(report) {
			_, _ = fmt.Fprintln(os.Stderr, line)
		}
		os.Exit(1)
	}
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
	_, _ = fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
