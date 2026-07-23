package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

var _ = Describe("release distribution workflow", func() {
	var workflow string

	BeforeEach(func() {
		raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release-distribution.yml"))
		Expect(err).NotTo(HaveOccurred())
		workflow = string(raw)
		var document map[string]any
		Expect(yaml.Unmarshal(raw, &document)).To(Succeed())
	})

	It("pins every third-party action to an immutable commit", func() {
		usesPattern := regexp.MustCompile(`(?m)^\s*uses:\s+[^@\s]+@([0-9a-f]{40})(?:\s+#.*)?$`)
		allUses := regexp.MustCompile(`(?m)^\s*uses:\s+.*$`).FindAllString(workflow, -1)
		pinnedUses := usesPattern.FindAllStringSubmatch(workflow, -1)

		Expect(pinnedUses).To(HaveLen(len(allUses)))
		Expect(workflow).NotTo(MatchRegexp(`uses:\s+[^@\s]+@v[0-9]`))
	})

	It("resolves one existing tag identity and checks out its exact source SHA", func() {
		for _, snippet := range []string{
			`^[0-9]+\.[0-9]+\.[0-9]+$`,
			`refs/ccp-release-tags/${TAG}^{commit}`,
			`source_sha=${SOURCE_SHA}`,
			`tag_oid=${TAG_OID}`,
			`ref: ${{ needs.preflight.outputs.source_sha }}`,
			`test "$(git rev-parse HEAD)" = "${EXPECTED_SHA}"`,
		} {
			Expect(workflow).To(ContainSubstring(snippet))
		}
		Expect(strings.Count(workflow, `test "$(git rev-parse "refs/ccp-release-tags/${TAG}^{commit}")" = "${EXPECTED_SHA}"`)).To(BeNumerically(">=", 3))
	})

	It("validates six canonical artifacts before draft smoke and publication", func() {
		for _, snippet := range []string{
			`goos: [linux, darwin, windows]`,
			`goarch: [amd64, arm64]`,
			`CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH"`,
			`go build -trimpath -ldflags "$ldflags"`,
			`test "$(unzip -Z1 "$asset")" = "$bin_name"`,
			`source_sha:$source_sha`,
			`test "$(find release -maxdepth 1 -type f -name '*.zip' | wc -l)" -eq 6`,
			`sha256sum ./*.zip > ./ccp_checksums.txt`,
			`draft: true`,
			`gh release download "$TAG"`,
			`gh release edit "$TAG" --repo "$REPO" --draft=false --latest`,
		} {
			Expect(workflow).To(ContainSubstring(snippet))
		}

		draft := strings.Index(workflow, "  create-draft:")
		smoke := strings.Index(workflow, "  smoke-draft:")
		publish := strings.Index(workflow, "  publish:")
		Expect(draft).To(BeNumerically(">", 0))
		Expect(smoke).To(BeNumerically(">", draft))
		Expect(publish).To(BeNumerically(">", smoke))
	})
})

var _ = Describe("validation workflow dependencies", func() {
	DescribeTable("using the Go module lock for validation tools",
		func(name string) {
			raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", name))
			Expect(err).NotTo(HaveOccurred())
			workflow := string(raw)
			var document map[string]any
			Expect(yaml.Unmarshal(raw, &document)).To(Succeed())

			Expect(workflow).To(ContainSubstring("go mod download"))
			Expect(workflow).NotTo(ContainSubstring("go install "))
			Expect(workflow).NotTo(ContainSubstring("raw.githubusercontent.com/golangci"))
		},
		Entry("main validation", "main-validation.yml"),
		Entry("pull-request validation", "pr-validation.yml"),
	)

	It("declares every executed validation tool in go.mod", func() {
		raw, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
		Expect(err).NotTo(HaveOccurred())
		module := string(raw)

		for _, tool := range []string{
			"github.com/fzipp/gocyclo/cmd/gocyclo",
			"github.com/golangci/golangci-lint/v2/cmd/golangci-lint",
			"github.com/gordonklaus/ineffassign",
			"golang.org/x/vuln/cmd/govulncheck",
			"honnef.co/go/tools/cmd/staticcheck",
		} {
			Expect(module).To(ContainSubstring("\n\t" + tool + "\n"))
		}
	})
})
