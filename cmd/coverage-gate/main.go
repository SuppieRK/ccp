package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"go-command-compression-proxy/internal/quality/coverage"
)

func main() {
	cfg := parseConfig()
	report := loadCoverageReport(cfg.coverProfile, cfg.modulePath, cfg.internalPrefix, cfg.threshold)
	out, summaryFile := prepareSummaryWriter(cfg.outPath)
	if err := writeSummary(out, report); err != nil {
		fail("write summary: %v", err)
	}
	if summaryFile != nil {
		defer func() {
			if err := summaryFile.Close(); err != nil {
				fail("close summary file: %v", err)
			}
		}()
		if err := summaryFile.Sync(); err != nil {
			fail("flush summary file: %v", err)
		}
	}
	if err := validateGate(report, cfg.threshold, cfg.internalPrefix); err != nil {
		fail("%v", err)
	}
}

type config struct {
	coverProfile   string
	modulePath     string
	internalPrefix string
	threshold      float64
	outPath        string
}

func parseConfig() config {
	cfg := config{}
	flag.StringVar(&cfg.coverProfile, "coverprofile", "", "path to go coverage profile")
	flag.StringVar(&cfg.modulePath, "module", "go-command-compression-proxy", "go module path")
	flag.StringVar(&cfg.internalPrefix, "internal-prefix", "internal/", "required package prefix")
	flag.Float64Var(&cfg.threshold, "threshold", 80.0, "required minimum coverage percentage")
	flag.StringVar(&cfg.outPath, "summary-out", "", "optional file path to write markdown summary")
	flag.Parse()
	if strings.TrimSpace(cfg.coverProfile) == "" {
		fail("coverprofile is required")
	}
	return cfg
}

func loadCoverageReport(coverProfile, modulePath, internalPrefix string, threshold float64) coverage.Report {
	f, err := os.Open(coverProfile)
	if err != nil {
		fail("open coverprofile: %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			fail("close coverprofile: %v", err)
		}
	}()

	report, err := coverage.ParseProfile(f, modulePath, internalPrefix, threshold)
	if err != nil {
		fail("parse profile: %v", err)
	}
	return report
}

func prepareSummaryWriter(outPath string) (io.Writer, *os.File) {
	if outPath == "" {
		return os.Stdout, nil
	}
	summaryFile, err := os.Create(outPath)
	if err != nil {
		fail("open summary output: %v", err)
	}
	return io.MultiWriter(os.Stdout, summaryFile), summaryFile
}

func renderSummary(report coverage.Report) string {
	var b strings.Builder
	_ = writeSummary(&b, report)
	return b.String()
}

func writeSummary(w io.Writer, report coverage.Report) error {
	ow := &orderedWriter{w: w}
	ow.writef("## Coverage Gate\n\n")
	ow.writef("Required scope: `%s` (threshold: %.2f%%)\n\n", report.InternalPrefix, report.Threshold)
	ow.writef(
		"Module-group coverage (`%s`): **%.2f%%** (%d/%d statements)\n\n",
		report.InternalPrefix,
		report.InternalTotal.Percent,
		report.InternalTotal.Covered,
		report.InternalTotal.Statements,
	)
	ow.writes("| Package | Coverage | Covered/Statements | Status |\n")
	ow.writes("|---|---:|---:|---|\n")
	for _, pkg := range report.InternalPackages {
		status := "PASS"
		if pkg.Percent < report.Threshold {
			status = "FAIL"
		}
		ow.writef("| `%s` | %.2f%% | %d/%d | %s |\n", pkg.Package, pkg.Percent, pkg.Covered, pkg.Statements, status)
	}
	ow.writes("\nInformational packages outside required scope:\n\n")
	if len(report.OtherPackages) == 0 {
		ow.writes("- none\n")
	} else {
		for _, pkg := range report.OtherPackages {
			ow.writef("- `%s`: %.2f%% (%d/%d)\n", pkg.Package, pkg.Percent, pkg.Covered, pkg.Statements)
		}
	}
	ow.writes("\n")
	return ow.err
}

type orderedWriter struct {
	w   io.Writer
	err error
}

func (ow *orderedWriter) writef(format string, args ...any) {
	if ow.err != nil {
		return
	}
	_, ow.err = fmt.Fprintf(ow.w, format, args...)
}

func (ow *orderedWriter) writes(s string) {
	if ow.err != nil {
		return
	}
	_, ow.err = io.WriteString(ow.w, s)
}

func fail(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func validateGate(report coverage.Report, threshold float64, internalPrefix string) error {
	if report.InternalTotal.Percent < threshold {
		return fmt.Errorf("coverage gate failed: %s total %.2f%% < %.2f%%", internalPrefix, report.InternalTotal.Percent, threshold)
	}

	failed := 0
	for _, pkg := range report.InternalPackages {
		if pkg.Percent < threshold {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("coverage gate failed: %d package(s) in %s below %.2f%%", failed, internalPrefix, threshold)
	}
	return nil
}
