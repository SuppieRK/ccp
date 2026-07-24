package main

import (
	"errors"
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
			`refs/cmdshape-release-tags/${TAG}^{commit}`,
			`source_sha=${SOURCE_SHA}`,
			`tag_oid=${TAG_OID}`,
			`ref: ${{ needs.preflight.outputs.source_sha }}`,
			`test "$(git rev-parse HEAD)" = "${EXPECTED_SHA}"`,
		} {
			Expect(workflow).To(ContainSubstring(snippet))
		}
		Expect(strings.Count(workflow, `test "$(git rev-parse "refs/cmdshape-release-tags/${TAG}^{commit}")" = "${EXPECTED_SHA}"`)).To(BeNumerically(">=", 3))
	})

	It("validates cmdshape-only artifacts before draft smoke and publication", func() {
		for _, snippet := range []string{
			`goos: [linux, darwin, windows]`,
			`goarch: [amd64, arm64]`,
			`CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH"`,
			`go build -trimpath -ldflags "$ldflags"`,
			`asset="cmdshape_${TAG}_${GOOS}_${GOARCH}.zip"`,
			`test "$(unzip -Z1 "$asset")" = "$cmdshape_bin"`,
			`source_sha:$source_sha`,
			`name '*.zip' | wc -l)" -eq 6`,
			`expected_binary="cmdshape"`,
			`expected_binary="cmdshape.exe"`,
			`test "$(jq -r .binary "$metadata_file")" = "$expected_binary"`,
			`test "$(unzip -Z1 "release/${asset}")" = "$expected_binary"`,
			`sha256sum ./cmdshape_*.zip > ./cmdshape_checksums.txt`,
			`test "$(wc -l < ./cmdshape_checksums.txt)" -eq 6`,
			`predicate-type: https://github.com/SuppieRK/cmdshape/attestations/binary-archive/v1`,
			`--pattern cmdshape_checksums.txt`,
			`./release/*_checksums.txt`,
			`draft: true`,
			`gh release download "$TAG"`,
			`gh release edit "$TAG" --repo "$REPO" --draft=false --latest`,
		} {
			Expect(workflow).To(ContainSubstring(snippet))
		}
		for _, forbidden := range []string{
			"ccp_checksums.txt",
			"distributions+=(ccp)",
			`$distributions += "ccp"`,
			`legacy_bin="ccp"`,
		} {
			Expect(workflow).NotTo(ContainSubstring(forbidden))
		}

		draft := strings.Index(workflow, "  create-draft:")
		smoke := strings.Index(workflow, "  smoke-draft:")
		publish := strings.Index(workflow, "  publish:")
		Expect(draft).To(BeNumerically(">", 0))
		Expect(smoke).To(BeNumerically(">", draft))
		Expect(publish).To(BeNumerically(">", smoke))
	})
})

var _ = Describe("hard cutover surfaces", func() {
	It("keeps the active product and distribution free of the retired identity", func() {
		repositoryRoot := filepath.Join("..", "..")
		legacyIdentity := regexp.MustCompile(`(?i)(^|[^a-z0-9_])ccp([^a-z0-9_]|$)|CCP_`)
		for _, name := range []string{
			"README.md",
			"AGENTS.md",
			"ARCHITECTURE.md",
			filepath.Join("docs", "agent-rules", "RELEASE.md"),
			filepath.Join("scripts", "install.sh"),
			filepath.Join(".github", "workflows", "release-distribution.yml"),
			filepath.Join("internal", "product", "identity.go"),
			filepath.Join("internal", "lifecycle", "upgrade.go"),
			filepath.Join("internal", "filtertrust", "trust.go"),
			filepath.Join("internal", "replay", "fixture.go"),
			filepath.Join("schemas", "cmdshape-filter.schema.json"),
		} {
			raw, err := os.ReadFile(filepath.Join(repositoryRoot, name))
			Expect(err).NotTo(HaveOccurred(), name)
			Expect(string(raw)).NotTo(MatchRegexp(legacyIdentity.String()), name)
		}

		for _, name := range []string{
			filepath.Join("cmd", "ccp"),
			filepath.Join("cmd", "ccp-ci"),
			filepath.Join("cmd", "ccp-docgen"),
		} {
			entries, err := os.ReadDir(filepath.Join(repositoryRoot, name))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			Expect(err).NotTo(HaveOccurred(), name)
			Expect(entries).To(BeEmpty(), name)
		}
		_, err := os.Stat(filepath.Join(repositoryRoot, "schemas", "ccp-filter.schema.json"))
		Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
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
