package coverage

import (
	"strings"
	"testing"
)

const (
	modulePathTest        = "go-command-compression-proxy"
	internalPrefixTest    = "internal/"
	parseProfileErrFmt    = "parse profile: %v"
	parseCoverProfileText = "parse coverprofile"
	repeatedCoverLine     = "go-command-compression-proxy/internal/runner/run.go:3.1,4.2 2 0"
)

func TestParseProfileBuildsInternalAndOtherStats(t *testing.T) {
	raw := strings.Join([]string{
		"mode: atomic",
		"go-command-compression-proxy/internal/runner/run.go:1.1,2.2 3 1",
		repeatedCoverLine,
		"go-command-compression-proxy/internal/engine/engine.go:1.1,2.2 5 1",
		"go-command-compression-proxy/cmd/ccp/main.go:1.1,2.2 4 1",
	}, "\n")

	report, err := ParseProfile(strings.NewReader(raw), modulePathTest, internalPrefixTest, 80)
	if err != nil {
		t.Fatalf(parseProfileErrFmt, err)
	}
	if got := len(report.InternalPackages); got != 2 {
		t.Fatalf("internal package count = %d, want 2", got)
	}
	if got := len(report.OtherPackages); got != 1 {
		t.Fatalf("other package count = %d, want 1", got)
	}
	if report.InternalTotal.Statements != 10 {
		t.Fatalf("internal statements = %d, want 10", report.InternalTotal.Statements)
	}
	if report.InternalTotal.Covered != 8 {
		t.Fatalf("internal covered = %d, want 8", report.InternalTotal.Covered)
	}
	if report.InternalTotal.Percent != 80 {
		t.Fatalf("internal percent = %.2f, want 80.00", report.InternalTotal.Percent)
	}
}

func TestParseProfileRejectsMalformedCoverageLines(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "malformed-line", raw: "mode: set\nbad-line\n"},
		{name: "invalid-statement-count", raw: "mode: set\ngo-command-compression-proxy/internal/runner/run.go:1.1,2.2 nope 1\n"},
		{name: "invalid-execution-count", raw: "mode: set\ngo-command-compression-proxy/internal/runner/run.go:1.1,2.2 3 nope\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseProfile(strings.NewReader(tc.raw), modulePathTest, internalPrefixTest, 80)
			if err == nil || !strings.Contains(err.Error(), parseCoverProfileText) {
				t.Fatalf("expected %s error, got: %v", parseCoverProfileText, err)
			}
		})
	}
}

func TestParseProfileWithNoInternalPackagesKeepsEmptyRequiredScope(t *testing.T) {
	raw := "mode: set\ngo-command-compression-proxy/cmd/ccp/main.go:1.1,2.2 4 1\n"
	report, err := ParseProfile(strings.NewReader(raw), modulePathTest, internalPrefixTest, 80)
	if err != nil {
		t.Fatalf(parseProfileErrFmt, err)
	}
	if len(report.InternalPackages) != 0 {
		t.Fatalf("internal packages = %d, want 0", len(report.InternalPackages))
	}
	if report.InternalTotal.Statements != 0 || report.InternalTotal.Covered != 0 || report.InternalTotal.Percent != 0 {
		t.Fatalf("internal total = %+v, want zero values", report.InternalTotal)
	}
	if got := len(report.OtherPackages); got != 1 {
		t.Fatalf("other package count = %d, want 1", got)
	}
}

func TestParseProfileDeduplicatesRepeatedBlocksFromCoverpkg(t *testing.T) {
	raw := strings.Join([]string{
		"mode: atomic",
		"go-command-compression-proxy/internal/runner/run.go:1.1,2.2 3 0",
		"go-command-compression-proxy/internal/runner/run.go:1.1,2.2 3 1",
		repeatedCoverLine,
		repeatedCoverLine,
	}, "\n")

	report, err := ParseProfile(strings.NewReader(raw), modulePathTest, internalPrefixTest, 80)
	if err != nil {
		t.Fatalf(parseProfileErrFmt, err)
	}
	if got := len(report.InternalPackages); got != 1 {
		t.Fatalf("internal package count = %d, want 1", got)
	}
	pkg := report.InternalPackages[0]
	if pkg.Package != "internal/runner" {
		t.Fatalf("package = %q, want internal/runner", pkg.Package)
	}
	if pkg.Statements != 5 {
		t.Fatalf("statements = %d, want 5", pkg.Statements)
	}
	if pkg.Covered != 3 {
		t.Fatalf("covered = %d, want 3", pkg.Covered)
	}
	if pkg.Percent != 60 {
		t.Fatalf("percent = %.2f, want 60.00", pkg.Percent)
	}
}

func TestParseProfileHandlesWindowsStylePaths(t *testing.T) {
	raw := strings.Join([]string{
		"mode: set",
		"go-command-compression-proxy\\internal\\runner\\run.go:1.1,2.2 2 1",
	}, "\n")
	report, err := ParseProfile(strings.NewReader(raw), modulePathTest, internalPrefixTest, 80)
	if err != nil {
		t.Fatalf(parseProfileErrFmt, err)
	}
	if got := len(report.InternalPackages); got != 1 {
		t.Fatalf("internal package count = %d, want 1", got)
	}
	if report.InternalPackages[0].Package != "internal/runner" {
		t.Fatalf("package = %q, want internal/runner", report.InternalPackages[0].Package)
	}
}
