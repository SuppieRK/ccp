package ci

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

var excludedGeneralSpecs = map[string]bool{
	"cli":           true,
	"command-proxy": true,
	"command-proxy-validation-and-benchmarking": true,
	"coverage-quality-gates":                    true,
	"github-artifact-attestations":              true,
	"init-claude-agent-integration":             true,
	"init-codex-agent-integration":              true,
	"init-integration-framework":                true,
	"init-opencode-agent-integration":           true,
	"installer-distribution":                    true,
	"metrics-audit-storage":                     true,
	"public-release-distribution":               true,
	"streaming-semantic-engine":                 true,
	"uv":                                        true,
	"cargo":                                     true, // parent router
	"docker":                                    true, // parent router
	"git":                                       true, // parent router
	"go":                                        true, // parent router
	"kubectl":                                   true, // parent router
	"npx":                                       true, // parent router
	"python":                                    true, // parent router only; benchmark lives under pytest tool coverage
}

func TestToolFixturesPresent(t *testing.T) {
	specNames := discoverSpecNames(t)
	missing := make([]string, 0)
	repoRoot := filepath.Join("..", "..", "..", "..")
	for _, spec := range specNames {
		p := filepath.Join(repoRoot, "testdata", "tool-fixtures", spec)
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing required spec fixture directories:\n%s", joinLines(missing))
	}
}

func TestBenchmarkFixturesCoverageNote(t *testing.T) {
	specNames := discoverSpecNames(t)
	missing := make([]string, 0)
	repoRoot := filepath.Join("..", "..", "..", "..")
	for _, spec := range specNames {
		p := filepath.Join(repoRoot, "testdata", "tool-fixtures", spec, "scenarios.json")
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		t.Logf("note: benchmark fixtures are still missing for some specs (eventual target):\n%s", joinLines(missing))
	}
}

func discoverSpecNames(t *testing.T) []string {
	t.Helper()
	specRoot := filepath.Join("..", "..", "..", "..", "openspec", "specs")
	entries, err := os.ReadDir(specRoot)
	if err != nil {
		t.Fatalf("read specs dir: %v", err)
	}
	specs := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if excludedGeneralSpecs[name] {
			continue
		}
		specMD := filepath.Join(specRoot, name, "spec.md")
		if st, err := os.Stat(specMD); err == nil && !st.IsDir() {
			specs = append(specs, name)
		}
	}
	sort.Strings(specs)
	return specs
}

func joinLines(items []string) string {
	sort.Strings(items)
	out := ""
	for _, item := range items {
		out += "- " + item + "\n"
	}
	return out
}
