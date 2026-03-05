package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go-command-compression-proxy/tools/benchmark/internal/ci"
)

func main() {
	var fixtures string
	var artifacts string
	var previousReport string
	var maxBytes int
	var tool string
	flag.StringVar(&fixtures, "fixtures-root", "testdata/tool-fixtures", "fixtures root")
	flag.StringVar(&artifacts, "artifacts-dir", ".artifacts/benchmark", "artifact output dir")
	flag.StringVar(&previousReport, "previous-report", "", "optional path to previous benchmark report.json")
	flag.IntVar(&maxBytes, "max-artifact-bytes", 5*1024*1024, "maximum bytes per artifact")
	flag.StringVar(&tool, "tool", "", "optional tool folder to run (relative to fixtures-root)")
	flag.Parse()
	if strings.TrimSpace(tool) != "" {
		fixtures = filepath.Join(fixtures, tool)
	}

	report, err := ci.Run(ci.RunOptions{
		FixturesRoot:     fixtures,
		ArtifactsDir:     artifacts,
		PreviousReport:   previousReport,
		MaxArtifactBytes: maxBytes,
		Timeout:          2 * time.Minute,
	})
	if err != nil {
		stderrln(err.Error())
		os.Exit(1)
	}
	if hasBenchmarkFailures(report) {
		printBenchmarkFailureReport(report)
		os.Exit(1)
	}
}

func hasBenchmarkFailures(report ci.RunReport) bool {
	return report.FailedRequired || report.FailedSafety
}

func printBenchmarkFailureReport(report ci.RunReport) {
	printFailureHeadings(report)
	for _, result := range report.Results {
		if isPassingScenario(result) {
			continue
		}
		printScenarioFailure(result)
	}
}

func printFailureHeadings(report ci.RunReport) {
	if report.FailedRequired {
		stderrln("required scenarios failed")
	}
	if report.FailedSafety {
		stderrln("safety-invariant scenarios failed")
	}
}

func isPassingScenario(result ci.ScenarioResult) bool {
	return result.SafetyPassed && (!result.Required || result.Success)
}

func printScenarioFailure(result ci.ScenarioResult) {
	stderrf("- scenario=%s tool=%s native_exit=%d proxy_exit=%d safety=%t\n",
		result.Scenario,
		result.Tool,
		result.Native.ExitCode,
		result.Proxy.ExitCode,
		result.SafetyPassed,
	)
	stderrf("  native=%q\n", result.Native.Spec)
	stderrf("  proxy=%q\n", result.Proxy.Spec)
	if len(result.Warnings) > 0 {
		stderrf("  warnings=%s\n", strings.Join(result.Warnings, "; "))
	}
}

func stderrln(msg string) {
	if _, err := fmt.Fprintln(os.Stderr, msg); err != nil {
		os.Exit(1)
	}
}

func stderrf(format string, args ...any) {
	if _, err := fmt.Fprintf(os.Stderr, format, args...); err != nil {
		os.Exit(1)
	}
}
