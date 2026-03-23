package yaml

import (
	"os"
	"path/filepath"

	"go-command-compression-proxy/internal/audit"
	"go-command-compression-proxy/internal/contracts"
	v2filters "go-command-compression-proxy/internal/filters"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("FilterSource", func() {
	It("resolves the repository source under the repo filters directory", func() {
		source := v2filters.RepositorySource("/repo")

		Expect(source).To(Equal(v2filters.FilterSource{
			Kind:      v2filters.SourceRepository,
			Directory: filepath.Join("/repo", "filters"),
		}))
	})

	It("resolves the project source under .ccp/filters", func() {
		source := v2filters.ProjectSource("/workspace/project")

		Expect(source).To(Equal(v2filters.FilterSource{
			Kind:      v2filters.SourceProject,
			Directory: filepath.Join("/workspace/project", ".ccp", "filters"),
		}))
	})

	It("resolves the home source under .config/ccp/filters", func() {
		source := v2filters.HomeSource("/home/suppie")

		Expect(source).To(Equal(v2filters.FilterSource{
			Kind:      v2filters.SourceHome,
			Directory: filepath.Join("/home/suppie", ".config", "ccp", "filters"),
		}))
	})
})

var _ = Describe("LoadRegistryFiltersFromSources", func() {
	var (
		root      string
		filterDir string
		auditHome string
	)

	BeforeEach(func() {
		var err error
		root, err = os.MkdirTemp("", "yaml-loader-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(root)).To(Succeed())
		})

		filterDir = filepath.Join(root, "filters")
		Expect(os.MkdirAll(filterDir, 0o755)).To(Succeed())

		auditHome, err = os.MkdirTemp("", "yaml-loader-audit-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(auditHome)).To(Succeed())
		})
	})

	It("builds generic operations-backed filters from authored YAML", func() {
		Expect(os.WriteFile(filepath.Join(filterDir, "python.yaml"), []byte(`
version: 1
filter: python
cases:
  - id: pytest
    when_arguments:
      have_sequence: [-m, pytest]
    compress_output:
      stdout:
        lines:
          keep:
            - regex: 'passed'
`), 0o644)).To(Succeed())

		filters, err := LoadRegistryFiltersFromSources([]v2filters.FilterSource{
			v2filters.RepositorySource(root),
		})
		Expect(err).NotTo(HaveOccurred())

		action := filters["python"].OnStdout("1 passed in 0.04s\n", yamlFilterContext{
			args: []string{"python", "-m", "pytest", "-q"},
		})
		Expect(action.Kind).To(Equal(contracts.ActionKeep))
	})

	It("skips invalid YAML scaffolds instead of failing registry construction", func() {
		Expect(os.WriteFile(filepath.Join(filterDir, ".mappings.yaml"), []byte(`
version: 1
map:
  npm: npm
`), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, "npm.yaml"), []byte(`
version: 1
filter: npm
about: Placeholder npm filter scaffold for YAML migration.
`), 0o644)).To(Succeed())

		filters, err := LoadRegistryFiltersFromSources([]v2filters.FilterSource{
			v2filters.RepositorySource(root),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(filters).NotTo(HaveKey("npm"))
	})

	It("records invalid definitions in the audit log", func() {
		restoreAudit := audit.WithTestConfig(auditHome, 8, 7)
		DeferCleanup(restoreAudit)
		DeferCleanup(audit.Reset)

		Expect(os.WriteFile(filepath.Join(filterDir, "broken.yaml"), []byte(`
version: 1
filter: broken
`), 0o644)).To(Succeed())

		_, err := LoadRegistryFiltersFromSources([]v2filters.FilterSource{
			v2filters.RepositorySource(root),
		})
		Expect(err).NotTo(HaveOccurred())

		auditData, err := os.ReadFile(filepath.Join(auditHome, ".config", "ccp", "audit", "audit.log"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(auditData)).To(ContainSubstring(`"msg":"filter_definition_invalid"`))
		Expect(string(auditData)).To(ContainSubstring(`broken.yaml`))
	})

	It("returns a stable error when a filter source path is a file", func() {
		sourceFile := filepath.Join(root, "not-a-dir")
		Expect(os.WriteFile(sourceFile, []byte("x"), 0o644)).To(Succeed())

		_, err := LoadRegistryFiltersFromSources([]v2filters.FilterSource{{Directory: sourceFile}})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(sourceFile))
		Expect(err.Error()).To(ContainSubstring("not a directory"))
	})

	It("registers mapped aliases for shipped-compatible filters", func() {
		Expect(os.WriteFile(filepath.Join(filterDir, ".mappings.yaml"), []byte(`
version: 1
map:
  pyright: basedpyright
`), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, "basedpyright.yaml"), []byte(`
version: 1
filter: basedpyright
cases:
  - id: diagnostics
    compress_output:
      stdout:
        lines:
          keep:
            - regex: 'error:'
`), 0o644)).To(Succeed())

		filters, err := LoadRegistryFiltersFromSources([]v2filters.FilterSource{
			v2filters.RepositorySource(root),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(HaveKey("basedpyright"))
		Expect(filters).To(HaveKey("pyright"))

		action := filters["pyright"].OnStdout("main.py:1:1 - error: boom\n", yamlFilterContext{
			args: []string{"pyright", "main.py"},
		})
		Expect(action.Kind).To(Equal(contracts.ActionKeep))
	})

	It("lets later files in the same source override duplicate filter ids deterministically", func() {
		Expect(os.WriteFile(filepath.Join(filterDir, "00-first.yaml"), []byte(`
version: 1
filter: demo
cases:
  - id: first
    compress_output:
      stdout:
        lines:
          keep:
            - contains: keep-me
`), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, "99-second.yaml"), []byte(`
version: 1
filter: demo
cases:
  - id: second
    compress_output:
      stdout:
        lines:
          replace:
            - regex: '^.*$'
              to: rewritten
`), 0o644)).To(Succeed())

		filters, err := LoadRegistryFiltersFromSources([]v2filters.FilterSource{
			v2filters.RepositorySource(root),
		})
		Expect(err).NotTo(HaveOccurred())

		// Intentional and documented override model: the later lexicographic file
		// within the same source wins, so duplicate ids behave as deterministic
		// overrides rather than loader errors.
		action := filters["demo"].OnStdout("keep-me\n", yamlFilterContext{args: []string{"demo"}})
		Expect(action.Kind).To(Equal(contracts.ActionReplace))
		Expect(action.Output).To(Equal("rewritten\n"))
	})
})

var _ = Describe("Shipped repository filters", func() {
	It("parse with strict validation from the real filters directory", func() {
		root := ProjectRootFromSource()
		paths, err := matchedFilterFiles(filepath.Join(root, "filters"))
		Expect(err).NotTo(HaveOccurred())
		Expect(paths).NotTo(BeEmpty())

		for _, path := range paths {
			raw, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred(), path)

			_, err = ParseDefinition(raw)
			Expect(err).NotTo(HaveOccurred(), path)
		}
	})
})

var _ = Describe("ProjectRootFromSource", func() {
	It("resolves to the repository root", func() {
		root := ProjectRootFromSource()
		Expect(root).NotTo(BeEmpty())
		Expect(filepath.IsAbs(root)).To(BeTrue())
		info, err := os.Stat(filepath.Join(root, "go.mod"))
		Expect(err).NotTo(HaveOccurred())
		Expect(info.IsDir()).To(BeFalse())
		entries := []string{"filters", "internal", "cmd"}
		for _, entry := range entries {
			stat, err := os.Stat(filepath.Join(root, entry))
			Expect(err).NotTo(HaveOccurred())
			Expect(stat.IsDir()).To(BeTrue())
		}
	})
})
