package coverage

import (
	"strings"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	modulePathTest        = "go-command-compression-proxy"
	internalPrefixTest    = "internal/"
	parseCoverProfileText = "parse coverprofile"
	repeatedCoverLine     = "go-command-compression-proxy/internal/runner/run.go:3.1,4.2 2 0"
)

var _ = ginkgo.Describe("ParseProfile", func() {
	ginkgo.It("builds internal and other package stats", func() {
		raw := strings.Join([]string{
			"mode: atomic",
			"go-command-compression-proxy/internal/runner/run.go:1.1,2.2 3 1",
			repeatedCoverLine,
			"go-command-compression-proxy/internal/engine/engine.go:1.1,2.2 5 1",
			"go-command-compression-proxy/cmd/ccp/main.go:1.1,2.2 4 1",
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
		ginkgo.Entry("invalid statement count", "mode: set\ngo-command-compression-proxy/internal/runner/run.go:1.1,2.2 nope 1\n"),
		ginkgo.Entry("invalid execution count", "mode: set\ngo-command-compression-proxy/internal/runner/run.go:1.1,2.2 3 nope\n"),
	)

	ginkgo.It("keeps the required scope empty when no internal packages are present", func() {
		raw := "mode: set\ngo-command-compression-proxy/cmd/ccp/main.go:1.1,2.2 4 1\n"

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
			"go-command-compression-proxy/internal/runner/run.go:1.1,2.2 3 0",
			"go-command-compression-proxy/internal/runner/run.go:1.1,2.2 3 1",
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

	ginkgo.It("handles Windows-style paths", func() {
		raw := strings.Join([]string{
			"mode: set",
			"go-command-compression-proxy\\internal\\runner\\run.go:1.1,2.2 2 1",
		}, "\n")

		report, err := ParseProfile(strings.NewReader(raw), modulePathTest, internalPrefixTest, 80)
		Expect(err).NotTo(HaveOccurred())
		Expect(report.InternalPackages).To(HaveLen(1))
		Expect(report.InternalPackages[0].Package).To(Equal("internal/runner"))
	})
})
