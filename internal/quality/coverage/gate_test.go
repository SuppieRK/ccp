package coverage

import (
	"strings"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	modulePathTest        = "github.com/SuppieRK/cmdshape"
	internalPrefixTest    = "internal/"
	parseCoverProfileText = "parse coverprofile"
	repeatedCoverLine     = "github.com/SuppieRK/cmdshape/internal/runner/run.go:3.1,4.2 2 0"
)

var _ = ginkgo.Describe("ParseProfile", func() {
	ginkgo.DescribeTable("rejecting missing required inputs",
		func(modulePath string, internalPrefix string, expected string) {
			_, err := ParseProfile(strings.NewReader("mode: set\n"), modulePath, internalPrefix, 80)
			Expect(err).To(MatchError(expected))
		},
		ginkgo.Entry("missing module path", "   ", internalPrefixTest, "module path is required"),
		ginkgo.Entry("missing internal prefix", modulePathTest, "   ", "internal prefix is required"),
	)

	ginkgo.It("builds internal and other package stats", func() {
		raw := strings.Join([]string{
			"mode: atomic",
			"github.com/SuppieRK/cmdshape/internal/runner/run.go:1.1,2.2 3 1",
			repeatedCoverLine,
			"github.com/SuppieRK/cmdshape/internal/engine/engine.go:1.1,2.2 5 1",
			"github.com/SuppieRK/cmdshape/cmd/cmdshape/main.go:1.1,2.2 4 1",
		}, "\n")

		report, err := ParseProfile(strings.NewReader(raw), modulePathTest, internalPrefixTest, 80)
		Expect(err).NotTo(HaveOccurred())
		Expect(report.InternalPackages).To(HaveLen(2))
		Expect(report.OtherPackages).To(HaveLen(1))
		Expect(report.InternalTotal.Statements).To(Equal(int64(10)))
		Expect(report.InternalTotal.Covered).To(Equal(int64(8)))
		Expect(report.InternalTotal.Percent).To(Equal(80.0))
	})

	ginkgo.DescribeTable("rejecting malformed coverage lines",
		func(raw string) {
			_, err := ParseProfile(strings.NewReader(raw), modulePathTest, internalPrefixTest, 80)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(parseCoverProfileText))
		},
		ginkgo.Entry("malformed line", "mode: set\nbad-line\n"),
		ginkgo.Entry("invalid statement count", "mode: set\ngithub.com/SuppieRK/cmdshape/internal/runner/run.go:1.1,2.2 nope 1\n"),
		ginkgo.Entry("invalid execution count", "mode: set\ngithub.com/SuppieRK/cmdshape/internal/runner/run.go:1.1,2.2 3 nope\n"),
	)

	ginkgo.It("keeps the required scope empty when no internal packages are present", func() {
		raw := "mode: set\ngithub.com/SuppieRK/cmdshape/cmd/cmdshape/main.go:1.1,2.2 4 1\n"

		report, err := ParseProfile(strings.NewReader(raw), modulePathTest, internalPrefixTest, 80)
		Expect(err).NotTo(HaveOccurred())
		Expect(report.InternalPackages).To(BeEmpty())
		Expect(report.InternalTotal.Statements).To(BeZero())
		Expect(report.InternalTotal.Covered).To(BeZero())
		Expect(report.InternalTotal.Percent).To(BeZero())
		Expect(report.OtherPackages).To(HaveLen(1))
	})

	ginkgo.It("deduplicates repeated blocks from coverpkg", func() {
		raw := strings.Join([]string{
			"mode: atomic",
			"github.com/SuppieRK/cmdshape/internal/runner/run.go:1.1,2.2 3 0",
			"github.com/SuppieRK/cmdshape/internal/runner/run.go:1.1,2.2 3 1",
			repeatedCoverLine,
			repeatedCoverLine,
		}, "\n")

		report, err := ParseProfile(strings.NewReader(raw), modulePathTest, internalPrefixTest, 80)
		Expect(err).NotTo(HaveOccurred())
		Expect(report.InternalPackages).To(HaveLen(1))
		pkg := report.InternalPackages[0]
		Expect(pkg.Package).To(Equal("internal/runner"))
		Expect(pkg.Statements).To(Equal(int64(5)))
		Expect(pkg.Covered).To(Equal(int64(3)))
		Expect(pkg.Percent).To(Equal(60.0))
	})

	ginkgo.It("keeps repeated uncovered blocks uncovered", func() {
		raw := strings.Join([]string{
			"mode: atomic",
			"github.com/SuppieRK/cmdshape/internal/runner/run.go:1.1,2.2 3 0",
			"github.com/SuppieRK/cmdshape/internal/runner/run.go:1.1,2.2 3 0",
		}, "\n")

		report, err := ParseProfile(strings.NewReader(raw), modulePathTest, internalPrefixTest, 80)
		Expect(err).NotTo(HaveOccurred())
		Expect(report.InternalPackages).To(HaveLen(1))
		Expect(report.InternalPackages[0].Covered).To(BeZero())
		Expect(report.InternalPackages[0].Statements).To(Equal(int64(3)))
		Expect(report.InternalPackages[0].Percent).To(BeZero())
	})

	ginkgo.It("keeps repeated covered blocks covered even when uncovered duplicates follow", func() {
		raw := strings.Join([]string{
			"mode: atomic",
			"github.com/SuppieRK/cmdshape/internal/runner/run.go:1.1,2.2 3 1",
			"github.com/SuppieRK/cmdshape/internal/runner/run.go:1.1,2.2 3 0",
			"github.com/SuppieRK/cmdshape/internal/runner/run.go:1.1,2.2 3 0",
		}, "\n")

		report, err := ParseProfile(strings.NewReader(raw), modulePathTest, internalPrefixTest, 80)
		Expect(err).NotTo(HaveOccurred())
		Expect(report.InternalPackages).To(HaveLen(1))
		Expect(report.InternalPackages[0].Covered).To(Equal(int64(3)))
		Expect(report.InternalPackages[0].Statements).To(Equal(int64(3)))
		Expect(report.InternalPackages[0].Percent).To(Equal(100.0))
	})

	ginkgo.It("handles Windows-style paths", func() {
		raw := strings.Join([]string{
			"mode: set",
			"github.com/SuppieRK/cmdshape\\internal\\runner\\run.go:1.1,2.2 2 1",
		}, "\n")

		report, err := ParseProfile(strings.NewReader(raw), modulePathTest, internalPrefixTest, 80)
		Expect(err).NotTo(HaveOccurred())
		Expect(report.InternalPackages).To(HaveLen(1))
		Expect(report.InternalPackages[0].Package).To(Equal("internal/runner"))
	})

	ginkgo.It("parses oversized coverage lines without hitting scanner limits", func() {
		longPath := modulePathTest + "/internal/" + strings.Repeat("very/deep/path/", 6000) + "run.go"
		raw := strings.Join([]string{
			"mode: set",
			longPath + ":1.1,2.2 2 1",
		}, "\n")

		report, err := ParseProfile(strings.NewReader(raw), modulePathTest, internalPrefixTest, 80)
		Expect(err).NotTo(HaveOccurred())
		Expect(report.InternalPackages).To(HaveLen(1))
		Expect(report.InternalPackages[0].Package).To(HavePrefix("internal/very/deep/path/"))
		Expect(report.InternalPackages[0].Covered).To(Equal(int64(2)))
		Expect(report.InternalPackages[0].Statements).To(Equal(int64(2)))
		Expect(report.InternalPackages[0].Percent).To(Equal(100.0))
	})
})

var _ = ginkgo.Describe("buildReport", func() {
	ginkgo.It("sorts internal and other packages independently", func() {
		report := buildReport(map[string]totals{
			"pkg/zeta":        {covered: 1, stmts: 2},
			"internal/zeta":   {covered: 3, stmts: 4},
			"internal/alpha":  {covered: 2, stmts: 2},
			"pkg/alpha":       {covered: 4, stmts: 5},
			"internal/middle": {covered: 1, stmts: 2},
			"pkg/middle":      {covered: 1, stmts: 3},
		}, modulePathTest, internalPrefixTest, 87.5)

		Expect(report.Threshold).To(Equal(87.5))
		Expect(report.InternalPackages).To(Equal([]PackageStat{
			{Package: "internal/alpha", Covered: 2, Statements: 2, Percent: 100},
			{Package: "internal/middle", Covered: 1, Statements: 2, Percent: 50},
			{Package: "internal/zeta", Covered: 3, Statements: 4, Percent: 75},
		}))
		Expect(report.OtherPackages).To(Equal([]PackageStat{
			{Package: "pkg/alpha", Covered: 4, Statements: 5, Percent: 80},
			{Package: "pkg/middle", Covered: 1, Statements: 3, Percent: 33.33},
			{Package: "pkg/zeta", Covered: 1, Statements: 2, Percent: 50},
		}))
	})
})

var _ = ginkgo.Describe("packageForFile", func() {
	ginkgo.DescribeTable("normalizing package paths",
		func(filePath string, expected string) {
			Expect(packageForFile(filePath, modulePathTest)).To(Equal(expected))
		},
		ginkgo.Entry("drops module-root files", "github.com/SuppieRK/cmdshape/main.go", ""),
		ginkgo.Entry("normalizes windows separators", `github.com/SuppieRK/cmdshape\internal\runner\run.go`, "internal/runner"),
		ginkgo.Entry("cleans nested relative segments", " github.com/SuppieRK/cmdshape/internal/runner/../engine/run.go ", "internal/engine"),
	)
})

var _ = ginkgo.Describe("percent", func() {
	ginkgo.DescribeTable("rounding coverage percentages",
		func(covered int64, statements int64, expected float64) {
			Expect(percent(covered, statements)).To(Equal(expected))
		},
		ginkgo.Entry("returns zero for empty totals", int64(0), int64(0), 0.0),
		ginkgo.Entry("rounds to two decimals", int64(1), int64(3), 33.33),
		ginkgo.Entry("preserves whole percentages", int64(4), int64(5), 80.0),
	)
})
