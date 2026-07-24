package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/SuppieRK/cmdshape/internal/quality/coverage"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	cfg, code, err := parseConfig(args)
	if err != nil {
		return writeFailure(stderr, code, "%v", err)
	}
	report, err := loadCoverageReport(cfg.coverProfile, cfg.modulePath, cfg.internalPrefix, cfg.threshold)
	if err != nil {
		return writeFailure(stderr, 1, "%v", err)
	}
	out, summaryFile, err := prepareSummaryWriter(stdout, cfg.outPath)
	if err != nil {
		return writeFailure(stderr, 1, "%v", err)
	}
	if summaryFile != nil {
		defer func() {
			if summaryFile != nil {
				_ = summaryFile.Close()
			}
		}()
	}
	if err := writeSummary(out, report); err != nil {
		return writeFailure(stderr, 1, "write summary: %v", err)
	}
	if summaryFile != nil {
		if err := summaryFile.Sync(); err != nil {
			return writeFailure(stderr, 1, "flush summary file: %v", err)
		}
		if err := summaryFile.Close(); err != nil {
			return writeFailure(stderr, 1, "close summary file: %v", err)
		}
		summaryFile = nil
	}
	if err := validateGate(report, cfg.threshold, cfg.internalPrefix); err != nil {
		return writeFailure(stderr, 1, "%v", err)
	}

	return 0
}

type config struct {
	coverProfile   string
	modulePath     string
	internalPrefix string
	threshold      float64
	outPath        string
}

func parseConfig(args []string) (config, int, error) {
	cfg := config{}
	flags := flag.NewFlagSet("coverage-gate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&cfg.coverProfile, "coverprofile", "", "path to go coverage profile")
	flags.StringVar(&cfg.modulePath, "module", "github.com/SuppieRK/cmdshape", "go module path")
	flags.StringVar(&cfg.internalPrefix, "internal-prefix", "internal/", "required package prefix")
	flags.Float64Var(&cfg.threshold, "threshold", 80.0, "required minimum coverage percentage")
	flags.StringVar(&cfg.outPath, "summary-out", "", "optional file path to write markdown summary")
	if err := flags.Parse(args); err != nil {
		return config{}, 2, err
	}
	if strings.TrimSpace(cfg.coverProfile) == "" {
		return config{}, 1, errors.New("coverprofile is required")
	}
	return cfg, 0, nil
}

func loadCoverageReport(coverProfile, modulePath, internalPrefix string, threshold float64) (report coverage.Report, err error) {
	f, err := os.Open(coverProfile)
	if err != nil {
		return coverage.Report{}, fmt.Errorf("open coverprofile: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close coverprofile: %w", closeErr)
		}
	}()

	report, err = coverage.ParseProfile(f, modulePath, internalPrefix, threshold)
	if err != nil {
		return coverage.Report{}, fmt.Errorf("parse profile: %w", err)
	}
	return report, nil
}

func prepareSummaryWriter(stdout io.Writer, outPath string) (io.Writer, *os.File, error) {
	if outPath == "" {
		return stdout, nil, nil
	}
	summaryFile, err := os.Create(outPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open summary output: %w", err)
	}
	return io.MultiWriter(stdout, summaryFile), summaryFile, nil
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

func writeFailure(w io.Writer, code int, format string, args ...any) int {
	_, _ = fmt.Fprintf(w, format+"\n", args...)
	return code
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
