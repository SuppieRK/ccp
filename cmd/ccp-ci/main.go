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
	flag.StringVar(&fixturesRoot, "fixtures-root", filepath.Join("testdata", "benchmarks"), "fixture root")
	flag.StringVar(&artifactsDir, "artifacts-dir", filepath.Join(".artifacts", "benchmark"), "artifact output dir")
	flag.StringVar(&tool, "tool", "", "optional tool directory to run")
	flag.Parse()

	if strings.TrimSpace(tool) != "" {
		fixturesRoot = filepath.Join(fixturesRoot, tool)
	}
	report, err := benchmark.Run(benchmark.RunOptions{
		FixturesRoot: fixturesRoot,
		ArtifactsDir: artifactsDir,
		ProxyBinary:  "ccp",
		Timeout:      2 * time.Minute,
	})
	if err != nil {
		fatal(err.Error())
	} else if report.Failed {
		for _, line := range benchmark.FailureSummary(report) {
			_, _ = fmt.Fprintln(os.Stderr, line)
		}
		os.Exit(1)
	}
}

func fatal(msg string) {
	_, _ = fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
