package main

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/quality/coverage"
)

const internalPrefix = "internal/"

func TestRenderSummaryIncludesStatuses(t *testing.T) {
	report := coverage.Report{
		InternalPrefix: internalPrefix,
		Threshold:      80,
		InternalTotal: coverage.PackageStat{
			Package:    internalPrefix,
			Covered:    8,
			Statements: 10,
			Percent:    80,
		},
		InternalPackages: []coverage.PackageStat{
			{Package: "internal/runner", Covered: 4, Statements: 5, Percent: 80},
			{Package: "internal/engine", Covered: 3, Statements: 5, Percent: 60},
		},
		OtherPackages: []coverage.PackageStat{
			{Package: "cmd/ccp", Covered: 2, Statements: 4, Percent: 50},
		},
	}

	out := renderSummary(report)
	if !strings.Contains(out, "Module-group coverage (`internal/`): **80.00%**") {
		t.Fatalf("expected module-group coverage line in summary:\n%s", out)
	}
	if !strings.Contains(out, "internal/runner") || !strings.Contains(out, "PASS") {
		t.Fatalf("expected pass package in summary:\n%s", out)
	}
	if !strings.Contains(out, "internal/engine") || !strings.Contains(out, "FAIL") {
		t.Fatalf("expected fail package in summary:\n%s", out)
	}
	if !strings.Contains(out, "| `internal/engine` | 60.00% | 3/5 | FAIL |") {
		t.Fatalf("expected failing package percentage/details in summary:\n%s", out)
	}
	if !strings.Contains(out, "Informational packages outside required scope") {
		t.Fatalf("expected informational section in summary:\n%s", out)
	}
}

func TestValidateGateFailsForLowModuleTotal(t *testing.T) {
	report := coverage.Report{
		InternalTotal: coverage.PackageStat{Percent: 79.99},
	}
	err := validateGate(report, 80.0, internalPrefix)
	if err == nil || !strings.Contains(err.Error(), "total 79.99% < 80.00%") {
		t.Fatalf("expected module total failure, got: %v", err)
	}
}

func TestValidateGateFailsForPackageThreshold(t *testing.T) {
	report := coverage.Report{
		InternalTotal: coverage.PackageStat{Percent: 85},
		InternalPackages: []coverage.PackageStat{
			{Package: "internal/a", Percent: 90},
			{Package: "internal/b", Percent: 79.5},
			{Package: "internal/c", Percent: 40},
		},
	}
	err := validateGate(report, 80.0, internalPrefix)
	if err == nil || !strings.Contains(err.Error(), "2 package(s) in internal/ below 80.00%") {
		t.Fatalf("expected package threshold failure, got: %v", err)
	}
}

func TestValidateGatePassesAtThreshold(t *testing.T) {
	report := coverage.Report{
		InternalTotal: coverage.PackageStat{Percent: 80},
		InternalPackages: []coverage.PackageStat{
			{Package: "internal/a", Percent: 80},
			{Package: "internal/b", Percent: 99.9},
		},
	}
	if err := validateGate(report, 80.0, internalPrefix); err != nil {
		t.Fatalf("expected pass at threshold, got: %v", err)
	}
}
