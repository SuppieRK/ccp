package yaml

import (
	"os"
	"path/filepath"
	"time"

	"go-command-compression-proxy/internal/audit"
	"go-command-compression-proxy/internal/contracts"
	v2filters "go-command-compression-proxy/internal/filters"
	"go-command-compression-proxy/internal/filtertrust"
	"go-command-compression-proxy/internal/version"

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
			Expect(cleanupAuditHome(auditHome)).To(Succeed())
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

	It("reports registry build timing per source", func() {
		Expect(os.WriteFile(filepath.Join(filterDir, "python.yaml"), []byte(`
version: 1
filter: python
cases:
  - id: pytest
    compress_output:
      stdout:
        lines:
          keep:
            - regex: '^'
`), 0o644)).To(Succeed())

		filters, timing, err := LoadRegistryFiltersFromSourcesWithTiming([]v2filters.FilterSource{
			v2filters.RepositorySource(root),
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(HaveKey("python"))
		Expect(timing.DurationMS).To(BeNumerically(">=", 0))
		Expect(timing.Sources).To(HaveLen(1))
		Expect(timing.Sources[0].SourceKind).To(Equal(string(v2filters.SourceRepository)))
		Expect(timing.Sources[0].SourceDir).To(Equal(filterDir))
		Expect(timing.Sources[0].Definitions).To(Equal(int64(1)))
		Expect(timing.Sources[0].Compiled).To(Equal(int64(1)))
		Expect(timing.Sources[0].DurationMS).To(BeNumerically(">=", 0))
		Expect(timing.Sources[0].Error).To(BeEmpty())
	})

	It("loads only the exact invoked filter on the execution path", func() {
		Expect(os.WriteFile(filepath.Join(filterDir, "git.yaml"), []byte(validLoaderStatusFilterYAML("git")), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, "pytest.yaml"), []byte(validLoaderStatusFilterYAML("pytest")), 0o644)).To(Succeed())

		filters, timing, err := LoadExecutionFilterFromSourcesWithTiming([]v2filters.FilterSource{
			v2filters.RepositorySource(root),
		}, "git")

		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(HaveKey("git"))
		Expect(filters).NotTo(HaveKey("pytest"))
		Expect(timing.Sources).To(HaveLen(1))
		Expect(timing.Sources[0].Definitions).To(Equal(int64(1)))
		Expect(timing.Sources[0].Compiled).To(Equal(int64(1)))
	})

	It("resolves aliases before loading their exact canonical filter", func() {
		Expect(os.WriteFile(filepath.Join(filterDir, ".mappings.yaml"), []byte("version: 1\nmap:\n  mvn: maven\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, "maven.yaml"), []byte(validLoaderStatusFilterYAML("maven")), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, "pytest.yaml"), []byte(validLoaderStatusFilterYAML("pytest")), 0o644)).To(Succeed())

		filters, timing, err := LoadExecutionFilterFromSourcesWithTiming([]v2filters.FilterSource{
			v2filters.RepositorySource(root),
		}, "mvn")

		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(HaveKey("mvn"))
		Expect(filters).NotTo(HaveKey("maven"))
		Expect(timing.Sources[0].Definitions).To(Equal(int64(1)))
		Expect(filters["mvn"].Dispatch(contracts.Command{Tool: "mvn", Args: []string{"mvn"}})).To(Equal("maven|passthrough"))
	})

	It("uses a compatibility scan only for a legacy arbitrary filename", func() {
		Expect(os.WriteFile(filepath.Join(filterDir, "legacy-name.yaml"), []byte(validLoaderStatusFilterYAML("git")), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, "other.yaml"), []byte(validLoaderStatusFilterYAML("pytest")), 0o644)).To(Succeed())

		filters, timing, err := LoadExecutionFilterFromSourcesWithTiming([]v2filters.FilterSource{
			v2filters.RepositorySource(root),
		}, "git")

		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(HaveKey("git"))
		Expect(timing.Sources[0].Definitions).To(Equal(int64(2)))
		Expect(timing.Sources[0].Compiled).To(Equal(int64(1)))
	})

	It("returns an empty targeted registry for an unknown command", func() {
		filters, timing, err := LoadExecutionFilterFromSourcesWithTiming([]v2filters.FilterSource{
			v2filters.RepositorySource(root),
		}, "unknown")

		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(BeEmpty())
		Expect(timing.Sources).To(HaveLen(1))
		Expect(timing.Sources[0].Compiled).To(BeZero())
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

var _ = Describe("DefaultSources", func() {
	It("uses only the repository source in dev builds", func() {
		prevVersion := version.Version
		version.Version = "dev"
		DeferCleanup(func() { version.Version = prevVersion })

		sources := DefaultSources()

		Expect(sources).To(Equal([]v2filters.FilterSource{
			v2filters.RepositorySource(ProjectRootFromSource()),
		}))
	})

	It("ignores an absent or untrusted project source in release builds", func() {
		prevVersion := version.Version
		version.Version = "1.2.3"
		DeferCleanup(func() { version.Version = prevVersion })

		home, err := os.UserHomeDir()
		Expect(err).NotTo(HaveOccurred())

		sources := DefaultSources()

		Expect(sources).To(Equal([]v2filters.FilterSource{
			v2filters.HomeSource(home),
		}))
	})

	It("uses an explicitly trusted project source before home in release builds", func() {
		prevVersion := version.Version
		version.Version = "1.2.3"
		DeferCleanup(func() { version.Version = prevVersion })
		project := GinkgoT().TempDir()
		home := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(project, ".ccp", "filters"), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(project, ".ccp", "filters", "git.yaml"), []byte(validLoaderStatusFilterYAML("git")), 0o644)).To(Succeed())
		previousCWD, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Chdir(project)).To(Succeed())
		DeferCleanup(func() { _ = os.Chdir(previousCWD) })
		previousHome := os.Getenv("HOME")
		Expect(os.Setenv("HOME", home)).To(Succeed())
		DeferCleanup(func() { _ = os.Setenv("HOME", previousHome) })
		restoreTrust := filtertrust.WithTestHome(home)
		DeferCleanup(restoreTrust)
		_, err = filtertrust.Trust(project)
		Expect(err).NotTo(HaveOccurred())

		sources := DefaultSources()

		Expect(sources).To(Equal([]v2filters.FilterSource{
			v2filters.ProjectSource(project),
			v2filters.HomeSource(home),
		}))
	})

	It("falls open to the home filter when project filter bytes are untrusted", func() {
		prevVersion := version.Version
		version.Version = "1.2.3"
		DeferCleanup(func() { version.Version = prevVersion })
		project := GinkgoT().TempDir()
		home := GinkgoT().TempDir()
		projectFilters := filepath.Join(project, ".ccp", "filters")
		homeFilters := filepath.Join(home, ".config", "ccp", "filters")
		Expect(os.MkdirAll(projectFilters, 0o755)).To(Succeed())
		Expect(os.MkdirAll(homeFilters, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(projectFilters, "git.yaml"), []byte(validLoaderStatusFilterYAML("git")), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(homeFilters, "git.yaml"), []byte(validLoaderStatusFilterYAML("git")), 0o644)).To(Succeed())
		previousCWD, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Chdir(project)).To(Succeed())
		DeferCleanup(func() { _ = os.Chdir(previousCWD) })
		previousHome := os.Getenv("HOME")
		Expect(os.Setenv("HOME", home)).To(Succeed())
		DeferCleanup(func() { _ = os.Setenv("HOME", previousHome) })
		restoreTrust := filtertrust.WithTestHome(home)
		DeferCleanup(restoreTrust)

		filters, err := LoadRegistryFiltersFromSources(DefaultSources())

		Expect(err).NotTo(HaveOccurred())
		provenance, ok := filters["git"].(contracts.ProvenanceFilter)
		Expect(ok).To(BeTrue())
		Expect(provenance.FilterProvenance().SourceKind).To(Equal(string(v2filters.SourceHome)))
	})
})

var _ = Describe("LoadRegistryStatusFromSources", func() {
	It("reports active, overridden, and broken entries while keeping runtime winners", func() {
		root := GinkgoT().TempDir()
		projectRoot := filepath.Join(root, "project")
		home := filepath.Join(root, "home")
		projectDir := filepath.Join(projectRoot, ".ccp", "filters")
		homeDir := filepath.Join(home, ".config", "ccp", "filters")
		Expect(os.MkdirAll(projectDir, 0o755)).To(Succeed())
		Expect(os.MkdirAll(homeDir, 0o755)).To(Succeed())

		Expect(os.WriteFile(filepath.Join(projectDir, "git.yaml"), []byte(validLoaderStatusFilterYAML("git")), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(projectDir, ".mappings.yaml"), []byte("version: 1\nmap:\n  gs: git\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(projectDir, "broken.yaml"), []byte("version: 1\nfilter: broken\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(homeDir, "git.yaml"), []byte(validLoaderStatusFilterYAML("git")), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(homeDir, ".mappings.yaml"), []byte("version: 1\nmap:\n  py: python\n"), 0o644)).To(Succeed())

		filters, rows, err := LoadRegistryStatusFromSources([]v2filters.FilterSource{
			v2filters.ProjectSource(projectRoot),
			v2filters.HomeSource(home),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(HaveKey("git"))
		Expect(filters).To(HaveKey("gs"))
		Expect(filters).NotTo(HaveKey("py"))

		Expect(rows).To(ContainElement(Equal(RegistryStatusRow{
			Tool:       "git",
			FilterPath: filepath.Join(projectDir, "git.yaml"),
			Target:     "",
			SourceKind: v2filters.SourceProject,
			Status:     "ok",
			order:      0,
		})))
		Expect(rows).To(ContainElement(Equal(RegistryStatusRow{
			Tool:       "gs",
			FilterPath: filepath.Join(projectDir, ".mappings.yaml"),
			Target:     "git",
			SourceKind: v2filters.SourceProject,
			Status:     "ok",
			order:      0,
		})))
		Expect(rows).To(ContainElement(Equal(RegistryStatusRow{
			Tool:       "git",
			FilterPath: filepath.Join(homeDir, "git.yaml"),
			Target:     "",
			SourceKind: v2filters.SourceHome,
			Status:     "overridden",
			order:      1,
		})))
		Expect(rows).To(ContainElement(Equal(RegistryStatusRow{
			Tool:       "py",
			FilterPath: filepath.Join(homeDir, ".mappings.yaml"),
			Target:     "python",
			SourceKind: v2filters.SourceHome,
			Status:     "missing target: python",
			order:      1,
		})))

		foundBroken := false
		for _, row := range rows {
			if row.Tool != "-" || row.FilterPath != filepath.Join(projectDir, "broken.yaml") || row.Target != "" || row.SourceKind != v2filters.SourceProject || row.order != 0 {
				continue
			}
			Expect(row.Status).To(ContainSubstring("invalid filter:"))
			foundBroken = true
			break
		}
		Expect(foundBroken).To(BeTrue())
	})

	It("returns status rows in stable priority and source order", func() {
		root := GinkgoT().TempDir()
		projectRoot := filepath.Join(root, "project")
		home := filepath.Join(root, "home")
		projectDir := filepath.Join(projectRoot, ".ccp", "filters")
		homeDir := filepath.Join(home, ".config", "ccp", "filters")
		Expect(os.MkdirAll(projectDir, 0o755)).To(Succeed())
		Expect(os.MkdirAll(homeDir, 0o755)).To(Succeed())

		Expect(os.WriteFile(filepath.Join(projectDir, "git.yaml"), []byte(validLoaderStatusFilterYAML("git")), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(projectDir, ".mappings.yaml"), []byte("version: 1\nmap:\n  gs: git\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(projectDir, "broken.yaml"), []byte("version: 1\nfilter: broken\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(homeDir, "git.yaml"), []byte(validLoaderStatusFilterYAML("git")), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(homeDir, ".mappings.yaml"), []byte("version: 1\nmap:\n  py: python\n"), 0o644)).To(Succeed())

		_, rows, err := LoadRegistryStatusFromSources([]v2filters.FilterSource{
			v2filters.ProjectSource(projectRoot),
			v2filters.HomeSource(home),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).To(HaveLen(5))
		Expect(rows[0]).To(Equal(RegistryStatusRow{
			Tool:       "git",
			FilterPath: filepath.Join(projectDir, "git.yaml"),
			SourceKind: v2filters.SourceProject,
			Status:     "ok",
			order:      0,
		}))
		Expect(rows[1]).To(Equal(RegistryStatusRow{
			Tool:       "gs",
			FilterPath: filepath.Join(projectDir, ".mappings.yaml"),
			Target:     "git",
			SourceKind: v2filters.SourceProject,
			Status:     "ok",
			order:      0,
		}))
		Expect(rows[2]).To(Equal(RegistryStatusRow{
			Tool:       "git",
			FilterPath: filepath.Join(homeDir, "git.yaml"),
			SourceKind: v2filters.SourceHome,
			Status:     "overridden",
			order:      1,
		}))
		Expect(rows[3].Tool).To(Equal("-"))
		Expect(rows[3].FilterPath).To(Equal(filepath.Join(projectDir, "broken.yaml")))
		Expect(rows[3].Target).To(Equal(""))
		Expect(rows[3].SourceKind).To(Equal(v2filters.SourceProject))
		Expect(rows[3].Status).To(ContainSubstring("invalid filter:"))
		Expect(rows[3].order).To(Equal(0))
		Expect(rows[4]).To(Equal(RegistryStatusRow{
			Tool:       "py",
			FilterPath: filepath.Join(homeDir, ".mappings.yaml"),
			Target:     "python",
			SourceKind: v2filters.SourceHome,
			Status:     "missing target: python",
			order:      1,
		}))
	})

	It("reports source-level errors without failing the overall status load", func() {
		root := GinkgoT().TempDir()
		sourceFile := filepath.Join(root, "not-a-dir")
		Expect(os.WriteFile(sourceFile, []byte("x"), 0o644)).To(Succeed())

		filters, rows, err := LoadRegistryStatusFromSources([]v2filters.FilterSource{{
			Kind:      v2filters.SourceHome,
			Directory: sourceFile,
		}})
		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(BeEmpty())
		foundSourceError := false
		for _, row := range rows {
			if row.Tool != "-" || row.FilterPath != sourceFile || row.Target != "" || row.SourceKind != v2filters.SourceHome {
				continue
			}
			Expect(row.Status).To(ContainSubstring("source error:"))
			foundSourceError = true
			break
		}
		Expect(foundSourceError).To(BeTrue())
	})
})

var _ = Describe("loader status helpers", func() {
	DescribeTable("assigns stable status priorities",
		func(status string, expected int) {
			Expect(statusPriority(status)).To(Equal(expected))
		},
		Entry("active filters sort first", "ok", 0),
		Entry("overridden filters sort after active filters", "overridden", 1),
		Entry("errors sort last", "missing target: demo", 2),
	)

	DescribeTable("compares source precedence",
		func(left, right, expected int) {
			Expect(compareSourceOrder(left, right)).To(Equal(expected))
		},
		Entry("lower order sorts before higher order", 0, 1, -1),
		Entry("higher order sorts after lower order", 2, 1, 1),
		Entry("matching order stays equal", 3, 3, 0),
	)

	DescribeTable("compares status rows deterministically",
		func(left, right RegistryStatusRow, expected int) {
			Expect(compareStatusRows(left, right)).To(Equal(expected))
		},
		Entry("status priority wins first",
			RegistryStatusRow{Tool: "demo", FilterPath: "/filters/demo.yaml", Status: "ok", order: 0},
			RegistryStatusRow{Tool: "demo", FilterPath: "/filters/demo.yaml", Status: "overridden", order: 0},
			-1,
		),
		Entry("source order breaks ties after status",
			RegistryStatusRow{Tool: "demo", FilterPath: "/filters/demo.yaml", Status: "ok", order: 0},
			RegistryStatusRow{Tool: "demo", FilterPath: "/filters/demo.yaml", Status: "ok", order: 1},
			-1,
		),
		Entry("tool name sorts lexicographically",
			RegistryStatusRow{Tool: "alpha", FilterPath: "/filters/demo.yaml", Status: "ok", order: 0},
			RegistryStatusRow{Tool: "beta", FilterPath: "/filters/demo.yaml", Status: "ok", order: 0},
			-1,
		),
		Entry("filter path sorts after tool name",
			RegistryStatusRow{Tool: "demo", FilterPath: "/filters/a.yaml", Status: "ok", order: 0},
			RegistryStatusRow{Tool: "demo", FilterPath: "/filters/b.yaml", Status: "ok", order: 0},
			-1,
		),
		Entry("target sorts last when the rest matches",
			RegistryStatusRow{Tool: "demo", FilterPath: "/filters/demo.yaml", Target: "alpha", Status: "ok", order: 0},
			RegistryStatusRow{Tool: "demo", FilterPath: "/filters/demo.yaml", Target: "beta", Status: "ok", order: 0},
			-1,
		),
		Entry("tool ordering returns a positive comparison in reverse lexicographic cases",
			RegistryStatusRow{Tool: "beta", FilterPath: "/filters/demo.yaml", Status: "ok", order: 0},
			RegistryStatusRow{Tool: "alpha", FilterPath: "/filters/demo.yaml", Status: "ok", order: 0},
			1,
		),
		Entry("target ordering returns a positive comparison in reverse lexicographic cases",
			RegistryStatusRow{Tool: "demo", FilterPath: "/filters/demo.yaml", Target: "beta", Status: "ok", order: 0},
			RegistryStatusRow{Tool: "demo", FilterPath: "/filters/demo.yaml", Target: "alpha", Status: "ok", order: 0},
			1,
		),
		Entry("identical rows compare equal",
			RegistryStatusRow{Tool: "demo", FilterPath: "/filters/demo.yaml", Target: "beta", Status: "ok", order: 0},
			RegistryStatusRow{Tool: "demo", FilterPath: "/filters/demo.yaml", Target: "beta", Status: "ok", order: 0},
			0,
		),
	)
})

var _ = Describe("registerCompiledFilterStatuses", func() {
	It("marks existing filters as overridden and emits rows in tool order", func() {
		registered := map[string]contracts.Filter{
			"alpha": nil,
		}
		rows := make([]RegistryStatusRow, 0)

		registerCompiledFilterStatuses(registered, map[string]compiledStatusFilter{
			"beta":  {tool: "beta", path: filepath.Join("/filters", "02-beta.yaml")},
			"alpha": {tool: "alpha", path: filepath.Join("/filters", "01-alpha.yaml")},
		}, v2filters.FilterSource{Kind: v2filters.SourceProject}, 2, &rows)

		Expect(rows).To(Equal([]RegistryStatusRow{
			{
				Tool:       "alpha",
				FilterPath: filepath.Join("/filters", "01-alpha.yaml"),
				SourceKind: v2filters.SourceProject,
				Status:     "overridden",
				order:      2,
			},
			{
				Tool:       "beta",
				FilterPath: filepath.Join("/filters", "02-beta.yaml"),
				SourceKind: v2filters.SourceProject,
				Status:     "ok",
				order:      2,
			},
		}))
		Expect(registered).To(HaveKey("alpha"))
		Expect(registered).To(HaveKey("beta"))
	})
})

var _ = Describe("registerMappedFilterStatuses", func() {
	var (
		root         string
		mappingsPath string
		source       v2filters.FilterSource
	)

	BeforeEach(func() {
		root = GinkgoT().TempDir()
		mappingsPath = filepath.Join(root, ".mappings.yaml")
		source = v2filters.FilterSource{
			Kind:      v2filters.SourceProject,
			Directory: root,
		}
	})

	It("reports invalid mapping files without mutating the registry", func() {
		Expect(os.WriteFile(mappingsPath, []byte("version: 2\nmap:\n  demo: target\n"), 0o644)).To(Succeed())

		registered := map[string]contracts.Filter{}
		rows := make([]RegistryStatusRow, 0)
		registerMappedFilterStatuses(registered, nil, source, 3, &rows)

		Expect(registered).To(BeEmpty())
		Expect(rows).To(HaveLen(1))
		Expect(rows[0].Tool).To(Equal("-"))
		Expect(rows[0].FilterPath).To(Equal(mappingsPath))
		Expect(rows[0].SourceKind).To(Equal(v2filters.SourceProject))
		Expect(rows[0].Status).To(ContainSubstring("invalid mappings:"))
		Expect(rows[0].order).To(Equal(3))
	})

	It("sorts aliases while distinguishing overridden and missing targets", func() {
		Expect(os.WriteFile(mappingsPath, []byte("version: 1\nmap:\n  gamma: missing\n  beta: demo\n  alpha: demo\n"), 0o644)).To(Succeed())

		registered := map[string]contracts.Filter{
			"alpha": nil,
		}
		rows := make([]RegistryStatusRow, 0)
		registerMappedFilterStatuses(registered, map[string]compiledStatusFilter{
			"demo": {tool: "demo", path: filepath.Join(root, "demo.yaml")},
		}, source, 1, &rows)

		Expect(rows).To(Equal([]RegistryStatusRow{
			{
				Tool:       "alpha",
				FilterPath: mappingsPath,
				Target:     "demo",
				SourceKind: v2filters.SourceProject,
				Status:     "overridden",
				order:      1,
			},
			{
				Tool:       "beta",
				FilterPath: mappingsPath,
				Target:     "demo",
				SourceKind: v2filters.SourceProject,
				Status:     "ok",
				order:      1,
			},
			{
				Tool:       "gamma",
				FilterPath: mappingsPath,
				Target:     "missing",
				SourceKind: v2filters.SourceProject,
				Status:     "missing target: missing",
				order:      1,
			},
		}))
		Expect(registered).To(HaveKey("alpha"))
		Expect(registered).To(HaveKey("beta"))
		Expect(registered).NotTo(HaveKey("gamma"))
	})
})

var _ = Describe("readMappingsFile", func() {
	var path string

	BeforeEach(func() {
		path = filepath.Join(GinkgoT().TempDir(), ".mappings.yaml")
	})

	It("returns an empty map when no mappings are declared", func() {
		Expect(os.WriteFile(path, []byte("version: 1\n"), 0o644)).To(Succeed())

		mappings, err := readMappingsFile(path)

		Expect(err).NotTo(HaveOccurred())
		Expect(mappings).To(BeEmpty())
	})

	It("trims aliases and targets before returning them", func() {
		Expect(os.WriteFile(path, []byte("version: 1\nmap:\n  \"  pyright  \": \"  basedpyright  \"\n"), 0o644)).To(Succeed())

		mappings, err := readMappingsFile(path)

		Expect(err).NotTo(HaveOccurred())
		Expect(mappings).To(Equal(map[string]string{"pyright": "basedpyright"}))
	})

	DescribeTable("rejects invalid mapping payloads",
		func(body, expected string) {
			Expect(os.WriteFile(path, []byte(body), 0o644)).To(Succeed())

			_, err := readMappingsFile(path)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(expected))
		},
		Entry("unsupported versions", "version: 2\nmap:\n  demo: target\n", "version must be exactly 1"),
		Entry("blank aliases after trimming", "version: 1\nmap:\n  \"  \": target\n", "mapping keys and values must be non-empty"),
		Entry("blank targets after trimming", "version: 1\nmap:\n  demo: \"   \"\n", "mapping keys and values must be non-empty"),
	)
})

var _ = Describe("loadFilterDefinitionsFromDir", func() {
	It("sorts loaded definitions by path while skipping invalid definitions", func() {
		root := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(root, "zeta.yaml"), []byte(validLoaderStatusFilterYAML("zeta")), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, "broken.yaml"), []byte("version: 1\nfilter: broken\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, "alpha.yaml"), []byte(validLoaderStatusFilterYAML("alpha")), 0o644)).To(Succeed())

		loaded, err := loadFilterDefinitionsFromDir(root)

		Expect(err).NotTo(HaveOccurred())
		Expect(loaded).To(HaveLen(2))
		Expect(loaded[0].Path).To(Equal(filepath.Join(root, "alpha.yaml")))
		Expect(loaded[0].Spec.Filter).To(Equal("alpha"))
		Expect(loaded[1].Path).To(Equal(filepath.Join(root, "zeta.yaml")))
		Expect(loaded[1].Spec.Filter).To(Equal("zeta"))
	})
})

var _ = Describe("compileFilters", func() {
	It("returns an empty registry when no definitions are loaded", func() {
		filters, err := compileFilters(nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(BeEmpty())
	})

	It("lets later loaded definitions override earlier duplicates deterministically", func() {
		stringPtr := func(v string) *string { return &v }
		loaded := []LoadedFilter{
			{
				Path: filepath.Join("/filters", "00-first.yaml"),
				Spec: &FilterDefinition{
					Version: 1,
					Filter:  "demo",
					Cases: []CaseClause{{
						ID: "first",
						CompressOutput: &OutputShape{
							Stdout: &OutputScope{
								Lines: &OutputLines{
									Keep: []SkipOrKeepRule{{Contains: "keep-me"}},
								},
							},
						},
					}},
				},
			},
			{
				Path: filepath.Join("/filters", "99-second.yaml"),
				Spec: &FilterDefinition{
					Version: 1,
					Filter:  "demo",
					Cases: []CaseClause{{
						ID: "second",
						CompressOutput: &OutputShape{
							Stdout: &OutputScope{
								Lines: &OutputLines{
									Replace: []ReplaceRule{{
										Regex: `^.*$`,
										To:    stringPtr("rewritten"),
									}},
								},
							},
						},
					}},
				},
			},
		}

		filters, err := compileFilters(loaded)

		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(HaveKey("demo"))
		action := filters["demo"].OnStdout("keep-me\n", yamlFilterContext{args: []string{"demo"}})
		Expect(action.Kind).To(Equal(contracts.ActionReplace))
		Expect(action.Output).To(Equal("rewritten\n"))
	})
})

var _ = Describe("matchedFilterFiles", func() {
	It("returns nil when the source directory does not exist", func() {
		paths, err := matchedFilterFiles(filepath.Join(GinkgoT().TempDir(), "missing"))

		Expect(err).NotTo(HaveOccurred())
		Expect(paths).To(BeNil())
	})

	It("returns only visible YAML files in sorted order", func() {
		root := GinkgoT().TempDir()
		Expect(os.Mkdir(filepath.Join(root, "nested"), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, "b.yaml"), []byte("version: 1\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, "a.yml"), []byte("version: 1\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, ".hidden.yaml"), []byte("version: 1\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, "notes.txt"), []byte("ignore"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, "nested", "child.yaml"), []byte("version: 1\n"), 0o644)).To(Succeed())

		paths, err := matchedFilterFiles(root)

		Expect(err).NotTo(HaveOccurred())
		Expect(paths).To(Equal([]string{
			filepath.Join(root, "a.yml"),
			filepath.Join(root, "b.yaml"),
		}))
	})
})

var _ = Describe("loadFilterDefinitionsFromDir", func() {
	It("returns no loaded filters when every discovered definition is invalid", func() {
		root := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(root, "broken-a.yaml"), []byte("version: 1\nfilter: broken-a\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, "broken-b.yaml"), []byte("version: 1\nfilter: broken-b\n"), 0o644)).To(Succeed())

		loaded, err := loadFilterDefinitionsFromDir(root)

		Expect(err).NotTo(HaveOccurred())
		Expect(loaded).To(BeEmpty())
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

func cleanupAuditHome(home string) error {
	audit.Reset()
	auditDir := filepath.Join(home, ".config", "ccp", "audit")
	var lastErr error
	for range 10 {
		if err := os.RemoveAll(auditDir); err == nil || os.IsNotExist(err) {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return lastErr
}

func validLoaderStatusFilterYAML(filterID string) string {
	return "version: 1\nfilter: " + filterID + "\ncases:\n  - id: passthrough\n    passthrough: true\n"
}
