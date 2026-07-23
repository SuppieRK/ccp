package lifecycle

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	filteryaml "go-command-compression-proxy/internal/filters/yaml"
	"go-command-compression-proxy/internal/filtertrust"
	"go-command-compression-proxy/internal/metrics"
	"go-command-compression-proxy/internal/version"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("filter", func() {
	captureStdout := func(fn func() error) string {
		orig := os.Stdout
		r, w, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		os.Stdout = w
		DeferCleanup(func() { os.Stdout = orig })

		Expect(fn()).To(Succeed())
		Expect(w.Close()).To(Succeed())

		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Close()).To(Succeed())
		return buf.String()
	}

	captureStderr := func(fn func() error) string {
		orig := os.Stderr
		r, w, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		os.Stderr = w
		DeferCleanup(func() { os.Stderr = orig })

		Expect(fn()).To(Succeed())
		Expect(w.Close()).To(Succeed())

		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Close()).To(Succeed())
		return buf.String()
	}

	setHomeDir := func(home string) {
		Expect(os.Setenv("HOME", home)).To(Succeed())
		DeferCleanup(func() { _ = os.Unsetenv("HOME") })
		if runtime.GOOS == "windows" {
			Expect(os.Setenv("USERPROFILE", home)).To(Succeed())
			DeferCleanup(func() { _ = os.Unsetenv("USERPROFILE") })
		}
	}

	It("renders root help output for help flags", func() {
		for _, args := range [][]string{{"--help"}, {"-h"}} {
			out := captureStderr(func() error { return RunFilter(args) })
			for _, part := range []string{
				"ccp filter - YAML filter authoring and inspection helpers",
				"Usage:",
				"Flags:",
				"Notes:",
				"ccp filter <subcommand> [args...]",
				"subcommands: new, performance, prompt, status",
				"agents creating or improving filters should start with 'ccp filter prompt [name]'",
			} {
				Expect(out).To(ContainSubstring(part))
			}
		}
	})

	It("rejects missing and unknown subcommands", func() {
		Expect(RunFilter(nil)).To(MatchError("missing filter subcommand"))
		Expect(RunFilter([]string{"unknown"})).To(MatchError(`unknown filter subcommand "unknown"`))
	})

	DescribeTable("rendering filter status helpers",
		func(tool string, row filteryaml.RegistryStatusRow, expectedTool string, expectedPath string) {
			Expect(displayFilterStatusTool(tool)).To(Equal(expectedTool))
			Expect(displayFilterStatusPath(row)).To(Equal(expectedPath))
		},
		Entry("shows a placeholder for blank tools", "", filteryaml.RegistryStatusRow{FilterPath: "filters/demo.yaml"}, "-", compactFilterStatusPath("filters/demo.yaml")),
		Entry("renders mapping targets inline", "git", filteryaml.RegistryStatusRow{FilterPath: "filters/.mappings.yaml", Target: "git"}, "git", compactFilterStatusPath("filters/.mappings.yaml")+" -> git"),
	)

	It("compacts paths relative to the current working directory", func() {
		tmp := GinkgoT().TempDir()
		prev, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Chdir(tmp)).To(Succeed())
		DeferCleanup(func() { _ = os.Chdir(prev) })

		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(compactFilterStatusPath(filepath.Join(cwd, ".ccp", "filters", "demo.yaml"))).To(Equal("." + string(filepath.Separator) + filepath.Join(".ccp", "filters", "demo.yaml")))
	})

	It("compacts paths relative to the home directory when they are outside the working tree", func() {
		tmp := GinkgoT().TempDir()
		home := filepath.Join(tmp, "home")
		work := filepath.Join(tmp, "work")
		prev, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.MkdirAll(home, 0o755)).To(Succeed())
		Expect(os.MkdirAll(work, 0o755)).To(Succeed())
		Expect(os.Chdir(work)).To(Succeed())
		DeferCleanup(func() { _ = os.Chdir(prev) })
		setHomeDir(home)

		Expect(compactFilterStatusPath(filepath.Join(home, ".config", "ccp", "filters", "demo.yaml"))).To(Equal("~" + string(filepath.Separator) + filepath.Join(".config", "ccp", "filters", "demo.yaml")))
	})

	DescribeTable("compacting explicit roots",
		func(path, root, prefix, expected string, expectedOK bool) {
			compact, ok := compactPathFromRoot(path, root, prefix)

			Expect(ok).To(Equal(expectedOK))
			Expect(compact).To(Equal(expected))
		},
		Entry("returns the prefix for an exact root match", "/repo", "/repo", ".", ".", true),
		Entry("prefixes child paths relative to the current directory", filepath.Join("/repo", "filters", "demo.yaml"), "/repo", ".", "."+string(filepath.Separator)+filepath.Join("filters", "demo.yaml"), true),
		Entry("prefixes child paths relative to the home directory", filepath.Join("/home", "user", "demo.yaml"), filepath.Join("/home", "user"), "~", "~"+string(filepath.Separator)+"demo.yaml", true),
		Entry("rejects paths outside the root", filepath.Join("/repo", "..", "elsewhere"), "/repo", ".", "", false),
		Entry("rejects blank roots", "/repo/demo.yaml", "   ", ".", "", false),
	)

	It("falls back to resolved symlink paths when direct relative compaction fails", func() {
		if runtime.GOOS == "windows" {
			Skip("symlink fallback is not portable on Windows runners")
		}

		tmp := GinkgoT().TempDir()
		realRoot := filepath.Join(tmp, "real")
		linkRoot := filepath.Join(tmp, "link")
		Expect(os.MkdirAll(filepath.Join(realRoot, "filters"), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(realRoot, "filters", "demo.yaml"), []byte("demo"), 0o644)).To(Succeed())
		Expect(os.Symlink(realRoot, linkRoot)).To(Succeed())

		compact, ok := compactPathFromRoot(filepath.Join(linkRoot, "filters", "demo.yaml"), filepath.Join(realRoot, "filters"), ".")

		Expect(ok).To(BeTrue())
		Expect(compact).To(Equal("." + string(filepath.Separator) + "demo.yaml"))
	})

	Context("new", func() {
		It("renders help output", func() {
			out := captureStderr(func() error { return RunFilter([]string{"new", "--help"}) })
			for _, part := range []string{
				"ccp filter new - generate a commented YAML scaffold for a new filter",
				"Usage:",
				"Flags:",
				"Notes:",
				"./.ccp/filters/<name>.yaml",
				".mappings.yaml",
			} {
				Expect(out).To(ContainSubstring(part))
			}
		})

		It("creates a valid scaffold and identity mapping", func() {
			tmp := GinkgoT().TempDir()
			prev, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Chdir(tmp)).To(Succeed())
			DeferCleanup(func() { _ = os.Chdir(prev) })

			Expect(RunFilter([]string{"new", "demo-tool"})).To(Succeed())

			filterPath := filepath.Join(tmp, ".ccp", "filters", "demo-tool.yaml")
			mappingsPath := filepath.Join(tmp, ".ccp", "filters", ".mappings.yaml")
			scaffold, err := os.ReadFile(filterPath)
			Expect(err).NotTo(HaveOccurred())
			mappings, err := os.ReadFile(mappingsPath)
			Expect(err).NotTo(HaveOccurred())

			Expect(string(scaffold)).To(ContainSubstring("version: 1"))
			Expect(string(scaffold)).To(ContainSubstring("# yaml-language-server: $schema=https://raw.githubusercontent.com/SuppieRK/ccp/refs/heads/main/schemas/ccp-filter.schema.json"))
			Expect(string(scaffold)).To(ContainSubstring("filter: demo-tool"))
			Expect(string(scaffold)).To(ContainSubstring("passthrough: true"))
			Expect(string(scaffold)).To(ContainSubstring("Authoring reference:"))
			Expect(string(scaffold)).To(ContainSubstring("Cases are evaluated in order"))
			Expect(string(mappings)).To(ContainSubstring("version: 1"))
			Expect(string(mappings)).To(ContainSubstring("demo-tool: demo-tool"))
		})

		It("preserves existing mappings and appends the new identity entry", func() {
			tmp := GinkgoT().TempDir()
			prev, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Chdir(tmp)).To(Succeed())
			DeferCleanup(func() { _ = os.Chdir(prev) })

			Expect(os.MkdirAll(filepath.Join(tmp, ".ccp", "filters"), 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmp, ".ccp", "filters", ".mappings.yaml"), []byte("version: 1\nmap:\n  npm: npm\n"), 0o644)).To(Succeed())

			Expect(RunFilter([]string{"new", "demo-tool"})).To(Succeed())

			mappings, err := os.ReadFile(filepath.Join(tmp, ".ccp", "filters", ".mappings.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(mappings)).To(ContainSubstring("npm: npm"))
			Expect(string(mappings)).To(ContainSubstring("demo-tool: demo-tool"))
		})

		It("repairs blank identity mappings instead of leaving them unusable", func() {
			tmp := GinkgoT().TempDir()
			prev, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Chdir(tmp)).To(Succeed())
			DeferCleanup(func() { _ = os.Chdir(prev) })

			Expect(os.MkdirAll(filepath.Join(tmp, ".ccp", "filters"), 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmp, ".ccp", "filters", ".mappings.yaml"), []byte("version: 1\nmap:\n  demo-tool: \"   \"\n"), 0o644)).To(Succeed())

			Expect(RunFilter([]string{"new", "demo-tool"})).To(Succeed())

			mappings, err := os.ReadFile(filepath.Join(tmp, ".ccp", "filters", ".mappings.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(mappings)).To(ContainSubstring("demo-tool: demo-tool"))
		})

		It("normalizes legacy mappings that omit the version field", func() {
			tmp := GinkgoT().TempDir()
			prev, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Chdir(tmp)).To(Succeed())
			DeferCleanup(func() { _ = os.Chdir(prev) })

			Expect(os.MkdirAll(filepath.Join(tmp, ".ccp", "filters"), 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmp, ".ccp", "filters", ".mappings.yaml"), []byte("map:\n  npm: npm\n"), 0o644)).To(Succeed())

			Expect(RunFilter([]string{"new", "demo-tool"})).To(Succeed())

			mappings, err := os.ReadFile(filepath.Join(tmp, ".ccp", "filters", ".mappings.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(mappings)).To(HavePrefix("version: 1\n"))
			Expect(string(mappings)).To(ContainSubstring("npm: npm"))
			Expect(string(mappings)).To(ContainSubstring("demo-tool: demo-tool"))
		})

		It("rejects invalid filter names", func() {
			Expect(RunFilter([]string{"new", "Demo Tool"})).To(MatchError(ContainSubstring("invalid filter name")))
		})

		It("rejects an existing scaffold", func() {
			tmp := GinkgoT().TempDir()
			prev, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Chdir(tmp)).To(Succeed())
			DeferCleanup(func() { _ = os.Chdir(prev) })

			Expect(os.MkdirAll(filepath.Join(tmp, ".ccp", "filters"), 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmp, ".ccp", "filters", "demo-tool.yaml"), []byte("version: 1\n"), 0o644)).To(Succeed())

			Expect(RunFilter([]string{"new", "demo-tool"})).To(MatchError(ContainSubstring("already exists")))
		})

		It("normalizes legacy mappings that omit the version field when ensuring identities", func() {
			path := filepath.Join(GinkgoT().TempDir(), ".mappings.yaml")
			Expect(os.WriteFile(path, []byte("map:\n  npm: npm\n"), 0o644)).To(Succeed())

			Expect(ensureIdentityFilterMapping(path, "demo-tool")).To(Succeed())

			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(HavePrefix("version: 1\n"))
			Expect(string(body)).To(ContainSubstring("npm: npm"))
			Expect(string(body)).To(ContainSubstring("demo-tool: demo-tool"))
		})

		It("surfaces malformed mappings files when ensuring identities", func() {
			path := filepath.Join(GinkgoT().TempDir(), ".mappings.yaml")
			Expect(os.WriteFile(path, []byte("map: [broken"), 0o644)).To(Succeed())

			err := ensureIdentityFilterMapping(path, "demo-tool")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("read mappings file"))
		})

		It("surfaces mapping write errors after writing the scaffold", func() {
			tmp := GinkgoT().TempDir()
			prev, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Chdir(tmp)).To(Succeed())
			DeferCleanup(func() { _ = os.Chdir(prev) })

			mappingsPath := filepath.Join(tmp, ".ccp", "filters", ".mappings.yaml")
			Expect(os.MkdirAll(mappingsPath, 0o755)).To(Succeed())

			err = RunFilter([]string{"new", "demo-tool"})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("read mappings file"))
			Expect(filepath.Join(tmp, ".ccp", "filters", "demo-tool.yaml")).To(BeAnExistingFile())
		})
	})

	Context("performance", func() {
		It("renders help output", func() {
			out := captureStderr(func() error { return RunFilter([]string{"performance", "--help"}) })
			for _, part := range []string{
				"ccp filter performance - show YAML filter and case performance",
				"Usage:",
				"Flags:",
				"ccp filter performance [--format text|json|csv]",
				"grouped by invoked tool, resolved filter, case",
				"Use --tool <tool> for focused improvements",
				"Pair this report with 'ccp filter prompt <name>'",
			} {
				Expect(out).To(ContainSubstring(part))
			}
		})

		It("renders performance rows and improvement hints", func() {
			path := filepath.Join(GinkgoT().TempDir(), "gain.db")
			appendFilterPerformanceMetrics(path)

			out := captureStdout(func() error {
				return RunFilterWithMetrics([]string{"performance", "--limit", "10"}, path)
			})

			Expect(out).To(ContainSubstring("ccp filter performance"))
			Expect(out).To(ContainSubstring("| TOOL"))
			Expect(out).To(ContainSubstring("| FILTER"))
			Expect(out).To(ContainSubstring("| CASE"))
			Expect(out).To(ContainSubstring("| py"))
			Expect(out).To(ContainSubstring("| python"))
			Expect(out).To(ContainSubstring("| pytest"))
			Expect(out).To(ContainSubstring("Hints"))
			Expect(out).To(ContainSubstring("review-case: python|pytest"))
			Expect(out).To(ContainSubstring("passthrough-opportunity: echo noisy"))
			Expect(out).To(ContainSubstring("Filter build"))
			Expect(out).To(ContainSubstring("builds=2"))
			Expect(out).To(ContainSubstring("| SOURCE"))
			Expect(out).To(ContainSubstring("| project"))
		})

		It("renders JSON rows and suggestions", func() {
			path := filepath.Join(GinkgoT().TempDir(), "gain.db")
			appendFilterPerformanceMetrics(path)

			out := captureStdout(func() error {
				return RunFilterWithMetrics([]string{"performance", "--format", "json"}, path)
			})

			Expect(out).To(ContainSubstring(`"dataset": "filter-performance"`))
			Expect(out).To(ContainSubstring(`"tool": "py"`))
			Expect(out).To(ContainSubstring(`"filter": "python"`))
			Expect(out).To(ContainSubstring(`"case": "pytest"`))
			Expect(out).To(ContainSubstring(`"filter_hash": "hash-a"`))
			Expect(out).To(ContainSubstring(`"build": {`))
			Expect(out).To(ContainSubstring(`"builds": 2`))
			Expect(out).To(ContainSubstring(`"build_rows": [`))
			Expect(out).To(ContainSubstring(`"kind": "review-case"`))
		})

		It("renders CSV rows and suggestions", func() {
			path := filepath.Join(GinkgoT().TempDir(), "gain.db")
			appendFilterPerformanceMetrics(path)

			out := captureStdout(func() error {
				return RunFilterWithMetrics([]string{"performance", "--format", "csv"}, path)
			})

			Expect(out).To(ContainSubstring("dataset,since,tool_filter,failed_filter,row_kind"))
			Expect(out).To(ContainSubstring("filter-performance,,,false,data,py,python,pytest"))
			Expect(out).To(ContainSubstring("filter-performance,,,false,build-summary"))
			Expect(out).To(ContainSubstring("filter-performance,,,false,build-source"))
			Expect(out).To(ContainSubstring("filter-performance,,,false,suggestion"))
		})

		It("aggregates matching global performance rows", func() {
			tmp := GinkgoT().TempDir()
			firstPath := filepath.Join(tmp, "first.db")
			secondPath := filepath.Join(tmp, "second.db")
			Expect(metrics.Append(firstPath, metrics.RunMetric{
				Tool:                  "py",
				Command:               "py -m pytest",
				Dispatch:              "python|pytest",
				RawBytes:              20,
				KeptBytes:             10,
				DurationMS:            20,
				FilterSourceKind:      "project",
				FilterPath:            "/repo/.ccp/filters/python.yaml",
				FilterHash:            "hash-a",
				RegistryBuildRecorded: true,
				RegistryBuildMS:       10,
				RegistrySources: []metrics.RegistrySourceBuildMetric{
					{SourceKind: "project", SourceDir: "/repo/.ccp/filters", Definitions: 1, Compiled: 1, DurationMS: 8},
				},
			})).To(Succeed())
			Expect(metrics.Append(secondPath, metrics.RunMetric{
				Tool:                  "py",
				Command:               "py -m pytest",
				Dispatch:              "python|pytest",
				RawBytes:              20,
				KeptBytes:             10,
				DurationMS:            40,
				FilterSourceKind:      "project",
				FilterPath:            "/repo/.ccp/filters/python.yaml",
				FilterHash:            "hash-a",
				RegistryBuildRecorded: true,
				RegistryBuildMS:       30,
				RegistrySources: []metrics.RegistrySourceBuildMetric{
					{SourceKind: "project", SourceDir: "/repo/.ccp/filters", Definitions: 2, Compiled: 2, DurationMS: 22},
				},
			})).To(Succeed())

			rows, err := queryGlobalPerformanceRows(&globalQuerySession{
				sources: []globalMetricsSource{
					{CWD: "/repo-a", MetricsPath: firstPath},
					{CWD: "/repo-b", MetricsPath: secondPath},
				},
				failures: map[string]globalQueryFailure{},
			}, metrics.QueryOptions{})

			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(1))
			Expect(rows[0].Tool).To(Equal("py"))
			Expect(rows[0].Filter).To(Equal("python"))
			Expect(rows[0].Case).To(Equal("pytest"))
			Expect(rows[0].Commands).To(Equal(int64(2)))
			Expect(rows[0].RawBytes).To(Equal(int64(40)))
			Expect(rows[0].KeptBytes).To(Equal(int64(20)))
			Expect(rows[0].AvgDurationMS).To(Equal(float64(30)))

			buildSummary, buildRows, err := queryGlobalRegistryBuild(&globalQuerySession{
				sources: []globalMetricsSource{
					{CWD: "/repo-a", MetricsPath: firstPath},
					{CWD: "/repo-b", MetricsPath: secondPath},
				},
				failures: map[string]globalQueryFailure{},
			}, metrics.QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(buildSummary.Builds).To(Equal(int64(2)))
			Expect(buildSummary.AvgDurationMS).To(Equal(float64(20)))
			Expect(buildSummary.MaxDurationMS).To(Equal(int64(30)))
			Expect(buildRows).To(HaveLen(1))
			Expect(buildRows[0].Definitions).To(Equal(int64(3)))
			Expect(buildRows[0].Compiled).To(Equal(int64(3)))
		})

		It("rejects positional arguments", func() {
			Expect(RunFilterWithMetrics([]string{"performance", "extra"}, "")).To(MatchError("filter performance does not accept positional arguments"))
		})
	})

	Context("prompt", func() {
		It("renders help output", func() {
			out := captureStderr(func() error { return RunFilter([]string{"prompt", "--help"}) })
			for _, part := range []string{
				"ccp filter prompt - print an embedded agent prompt for creating or improving filters",
				"Usage:",
				"Flags:",
				"Notes:",
				"ccp filter prompt [name]",
				"embedded in the ccp binary",
				"copy global filters into ./.ccp/filters before editing",
			} {
				Expect(out).To(ContainSubstring(part))
			}
		})

		It("prints the generic embedded authoring prompt", func() {
			out := captureStdout(func() error { return RunFilter([]string{"prompt"}) })

			Expect(out).To(ContainSubstring("# CCP Filter Authoring Prompt"))
			Expect(out).To(ContainSubstring("You are helping create or improve a CCP YAML filter for `<filter-id>`."))
			Expect(out).To(ContainSubstring("When using the generic prompt, `<filter-id>` is a placeholder."))
			Expect(out).To(ContainSubstring("Start by working in the project-local filter directory: `./.ccp/filters`."))
			Expect(out).To(ContainSubstring("copy it into `./.ccp/filters` first and edit that project-local copy"))
			Expect(out).To(ContainSubstring("Do not edit global/home filters under `~/.config/ccp/filters` unless the user directly asks"))
			Expect(out).To(ContainSubstring("Do not edit shipped built-in filters under `filters/` unless the user directly asks"))
			Expect(out).To(ContainSubstring("ccp filter performance --tool <tool> --limit 30"))
			Expect(out).To(ContainSubstring("The `--tool` value is the invoked tool name"))
			Expect(out).To(ContainSubstring("`review-case` hints"))
			Expect(out).To(ContainSubstring("`failure-heavy` hints"))
			Expect(out).To(ContainSubstring("`passthrough-opportunity` hints"))
			Expect(out).To(ContainSubstring("Read `RUNS` as frequency"))
			Expect(out).To(ContainSubstring("`PASS` as passthrough rate"))
			Expect(out).To(ContainSubstring("ccp filter new <filter-id>"))
			Expect(out).To(ContainSubstring("ccp capture -- <tool> <args...>"))
			Expect(out).To(ContainSubstring("./.ccp/filters/<filter-id>.yaml"))
			Expect(out).To(ContainSubstring("ccp verify --dir <fixture-dir>"))
			Expect(out).NotTo(ContainSubstring("{{FILTER_ID}}"))
			Expect(out).NotTo(ContainSubstring("{{COMMAND_EXAMPLE}}"))
			Expect(out).NotTo(ContainSubstring("my-tool"))
		})

		It("personalizes the embedded prompt for a valid filter id", func() {
			out := captureStdout(func() error { return RunFilter([]string{"prompt", "demo-tool"}) })

			Expect(out).To(ContainSubstring("You are helping create or improve a CCP YAML filter for `demo-tool`."))
			Expect(out).To(ContainSubstring("ccp filter new demo-tool"))
			Expect(out).To(ContainSubstring("ccp capture -- demo-tool <args...>"))
			Expect(out).To(ContainSubstring("./.ccp/filters/demo-tool.yaml"))
			Expect(out).NotTo(ContainSubstring("ccp filter new my-tool"))
		})

		It("rejects invalid filter ids", func() {
			Expect(RunFilter([]string{"prompt", "Demo Tool"})).To(MatchError(ContainSubstring("invalid filter name")))
		})

		It("rejects too many arguments", func() {
			Expect(RunFilter([]string{"prompt", "demo", "extra"})).To(MatchError("expected at most one filter name"))
		})

		It("does not read repo-local filter documentation to render the prompt", func() {
			tmp := GinkgoT().TempDir()
			prev, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Chdir(tmp)).To(Succeed())
			DeferCleanup(func() { _ = os.Chdir(prev) })

			out := captureStdout(func() error { return RunFilter([]string{"prompt", "demo-tool"}) })

			Expect(out).To(ContainSubstring("You are helping create or improve a CCP YAML filter for `demo-tool`."))
			Expect(filepath.Join(tmp, "docs", "agent-rules", "FILTERS.md")).NotTo(BeAnExistingFile())
		})
	})

	Context("status", func() {
		BeforeEach(func() {
			oldVersion := version.Version
			version.Version = "1.2.3"
			DeferCleanup(func() { version.Version = oldVersion })
		})

		It("renders help output", func() {
			out := captureStderr(func() error { return RunFilter([]string{"status", "--help"}) })
			for _, part := range []string{
				"ccp filter status - show active, overridden, and broken filter registrations",
				"Usage:",
				"Flags:",
				"Notes:",
				"project-local filters override home-scoped filters",
				"agents creating or improving filters should run 'ccp filter prompt [name]'",
			} {
				Expect(out).To(ContainSubstring(part))
			}
		})

		It("renders active, overridden, and broken filter rows", func() {
			tmp := GinkgoT().TempDir()
			home := filepath.Join(tmp, "home")
			project := filepath.Join(tmp, "repo")
			prev, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.MkdirAll(filepath.Join(project, ".ccp", "filters"), 0o755)).To(Succeed())
			Expect(os.MkdirAll(filepath.Join(home, ".config", "ccp"), 0o755)).To(Succeed())
			Expect(os.Chdir(project)).To(Succeed())
			DeferCleanup(func() { _ = os.Chdir(prev) })
			setHomeDir(home)

			projectFilters := filepath.Join(project, ".ccp", "filters")
			homeFilters := filepath.Join(home, ".config", "ccp", "filters")
			Expect(os.WriteFile(filepath.Join(projectFilters, "git.yaml"), []byte(validStatusFilterYAML("git")), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(projectFilters, "broken.yaml"), []byte("version: 1\nfilter: broken\n"), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(projectFilters, ".mappings.yaml"), []byte("version: 1\nmap:\n  gs: git\n"), 0o644)).To(Succeed())

			Expect(os.WriteFile(homeFilters, []byte("not-a-directory"), 0o644)).To(Succeed())

			out := captureStdout(func() error { return RunFilter([]string{"status"}) })

			Expect(out).To(ContainSubstring("ccp filter status\n\nproject trust: untrusted (.)\n\nshowing 4 rows"))
			Expect(out).To(ContainSubstring("| TOOL"))
			Expect(out).To(ContainSubstring("| FILTER"))
			Expect(out).To(ContainSubstring("| SOURCE"))
			Expect(out).To(ContainSubstring("| STATUS"))
			Expect(out).To(ContainSubstring("| git"))
			Expect(out).To(ContainSubstring(compactFilterStatusPath(filepath.Join(projectFilters, "git.yaml"))))
			Expect(out).To(ContainSubstring("| gs"))
			Expect(out).To(ContainSubstring(displayFilterStatusPath(filteryaml.RegistryStatusRow{
				FilterPath: filepath.Join(projectFilters, ".mappings.yaml"),
				Target:     "git",
			})))
			Expect(out).To(ContainSubstring(compactFilterStatusPath(filepath.Join(projectFilters, "broken.yaml"))))
			Expect(out).To(ContainSubstring("invalid filter:"))
			Expect(out).To(ContainSubstring("untrusted"))
			Expect(out).To(ContainSubstring("| -"))
			Expect(out).To(ContainSubstring(compactFilterStatusPath(homeFilters)))
			Expect(out).To(ContainSubstring("source error:"))
			Expect(out).To(ContainSubstring("Next: run `ccp filter prompt <filter-id>` for the embedded agent workflow"))
		})

		It("shows project overrides and mapping target failures", func() {
			tmp := GinkgoT().TempDir()
			home := filepath.Join(tmp, "home")
			project := filepath.Join(tmp, "repo")
			prev, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.MkdirAll(filepath.Join(project, ".ccp", "filters"), 0o755)).To(Succeed())
			Expect(os.MkdirAll(filepath.Join(home, ".config", "ccp", "filters"), 0o755)).To(Succeed())
			Expect(os.Chdir(project)).To(Succeed())
			DeferCleanup(func() { _ = os.Chdir(prev) })
			setHomeDir(home)

			projectFilters := filepath.Join(project, ".ccp", "filters")
			homeFilters := filepath.Join(home, ".config", "ccp", "filters")
			Expect(os.WriteFile(filepath.Join(projectFilters, "git.yaml"), []byte(validStatusFilterYAML("git")), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(homeFilters, "git.yaml"), []byte(validStatusFilterYAML("git")), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(homeFilters, ".mappings.yaml"), []byte("version: 1\nmap:\n  py: python\n"), 0o644)).To(Succeed())

			out := captureStdout(func() error { return RunFilter([]string{"status"}) })

			Expect(out).To(ContainSubstring("showing 3 rows"))
			Expect(out).To(ContainSubstring(compactFilterStatusPath(filepath.Join(projectFilters, "git.yaml"))))
			Expect(out).To(ContainSubstring("ok"))
			Expect(out).To(ContainSubstring(compactFilterStatusPath(filepath.Join(homeFilters, "git.yaml"))))
			Expect(out).To(ContainSubstring("untrusted"))
			Expect(out).To(ContainSubstring(truncateTailForDisplay(displayFilterStatusPath(filteryaml.RegistryStatusRow{
				FilterPath: filepath.Join(homeFilters, ".mappings.yaml"),
				Target:     "python",
			}), 38)))
			Expect(out).To(ContainSubstring("missing target: python"))
		})

		It("renders a friendly empty state", func() {
			tmp := GinkgoT().TempDir()
			home := filepath.Join(tmp, "home")
			project := filepath.Join(tmp, "repo")
			prev, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.MkdirAll(project, 0o755)).To(Succeed())
			Expect(os.MkdirAll(home, 0o755)).To(Succeed())
			Expect(os.Chdir(project)).To(Succeed())
			DeferCleanup(func() { _ = os.Chdir(prev) })
			setHomeDir(home)

			out := captureStdout(func() error { return RunFilter([]string{"status"}) })

			Expect(strings.TrimSpace(out)).To(Equal("ccp filter status\n\nproject trust: absent (.)\n\nNo filters found.\n\nNext: run `ccp filter prompt <filter-id>` for the embedded agent workflow before editing or creating filters."))
		})
	})

	Context("trust", func() {
		BeforeEach(func() {
			oldVersion := version.Version
			version.Version = "1.2.3"
			DeferCleanup(func() { version.Version = oldVersion })
		})

		It("approves, detects changes, and removes approval for the current canonical project", func() {
			tmp := GinkgoT().TempDir()
			home := filepath.Join(tmp, "home")
			project := filepath.Join(tmp, "repo")
			filters := filepath.Join(project, ".ccp", "filters")
			Expect(os.MkdirAll(filters, 0o755)).To(Succeed())
			Expect(os.MkdirAll(home, 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(filters, "git.yaml"), []byte(validStatusFilterYAML("git")), 0o644)).To(Succeed())
			previous, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Chdir(project)).To(Succeed())
			DeferCleanup(func() { _ = os.Chdir(previous) })
			setHomeDir(home)

			out := captureStdout(func() error { return RunFilter([]string{"trust"}) })
			Expect(out).To(ContainSubstring("ccp filter trust: trusted " + project))
			status := captureStdout(func() error { return RunFilter([]string{"status"}) })
			Expect(status).To(ContainSubstring("project trust: trusted (.)"))
			Expect(status).To(ContainSubstring("| git"))
			Expect(status).To(ContainSubstring("| ok"))

			Expect(os.WriteFile(filepath.Join(filters, "git.yaml"), []byte(validStatusFilterYAML("git")+"# changed\n"), 0o644)).To(Succeed())
			status = captureStdout(func() error { return RunFilter([]string{"status"}) })
			Expect(status).To(ContainSubstring("project trust: changed (.)"))
			Expect(status).To(ContainSubstring("| changed"))

			out = captureStdout(func() error { return RunFilter([]string{"untrust"}) })
			Expect(out).To(ContainSubstring("ccp filter untrust: removed approval"))
			status = captureStdout(func() error { return RunFilter([]string{"status"}) })
			Expect(status).To(ContainSubstring("project trust: untrusted (.)"))
		})

		It("preserves approval through managed home repair", func() {
			tmp := GinkgoT().TempDir()
			home := filepath.Join(tmp, "home")
			project := filepath.Join(tmp, "repo")
			filters := filepath.Join(project, ".ccp", "filters")
			Expect(os.MkdirAll(filters, 0o755)).To(Succeed())
			Expect(os.MkdirAll(home, 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(filters, "git.yaml"), []byte(validStatusFilterYAML("git")), 0o644)).To(Succeed())
			previous, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Chdir(project)).To(Succeed())
			DeferCleanup(func() { _ = os.Chdir(previous) })
			setHomeDir(home)
			Expect(RunFilter([]string{"trust"})).To(Succeed())

			Expect(RunRepair([]string{"--yes"})).To(Succeed())

			decision, err := filtertrust.Evaluate(project)
			Expect(err).NotTo(HaveOccurred())
			Expect(decision.State).To(Equal(filtertrust.StateTrusted))
		})

		DescribeTable("rejects positional project targets",
			func(subcommand string) {
				Expect(RunFilter([]string{subcommand, "/other"})).To(MatchError(ContainSubstring("does not accept positional arguments")))
			},
			Entry("trust", "trust"),
			Entry("untrust", "untrust"),
		)
	})
})

func validStatusFilterYAML(filterID string) string {
	return "version: 1\nfilter: " + filterID + "\ncases:\n  - id: passthrough\n    passthrough: true\n"
}

func appendFilterPerformanceMetrics(path string) {
	Expect(metrics.Append(path, metrics.RunMetric{
		Tool:                  "py",
		Command:               "py -m pytest",
		Dispatch:              "python|pytest",
		RawBytes:              20,
		KeptBytes:             20,
		DurationMS:            25,
		FilterSourceKind:      "project",
		FilterPath:            "/repo/.ccp/filters/python.yaml",
		FilterHash:            "hash-a",
		RegistryBuildRecorded: true,
		RegistryBuildMS:       10,
		RegistrySources: []metrics.RegistrySourceBuildMetric{
			{SourceKind: "project", SourceDir: "/repo/.ccp/filters", Definitions: 2, Compiled: 2, DurationMS: 8},
		},
	})).To(Succeed())
	Expect(metrics.Append(path, metrics.RunMetric{
		Tool:                  "echo",
		Command:               "echo noisy",
		Dispatch:              "echo",
		RawBytes:              10,
		KeptBytes:             10,
		Passthrough:           true,
		RegistryBuildRecorded: true,
		RegistryBuildMS:       20,
		RegistrySources: []metrics.RegistrySourceBuildMetric{
			{SourceKind: "project", SourceDir: "/repo/.ccp/filters", Definitions: 2, Compiled: 2, DurationMS: 12},
		},
	})).To(Succeed())
	Expect(metrics.Append(path, metrics.RunMetric{
		Tool:        "echo",
		Command:     "echo noisy",
		Dispatch:    "echo",
		RawBytes:    10,
		KeptBytes:   10,
		Passthrough: true,
	})).To(Succeed())
}
