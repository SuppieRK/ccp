package lifecycle

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	filteryaml "go-command-compression-proxy/internal/filters/yaml"
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
				"subcommands: new, status",
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

		Expect(compactFilterStatusPath(filepath.Join(tmp, ".ccp", "filters", "demo.yaml"))).To(Equal("." + string(filepath.Separator) + filepath.Join(".ccp", "filters", "demo.yaml")))
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

			Expect(out).To(ContainSubstring("ccp filter status\n\nshowing 4 rows"))
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
			Expect(out).To(ContainSubstring("| -"))
			Expect(out).To(ContainSubstring(compactFilterStatusPath(homeFilters)))
			Expect(out).To(ContainSubstring("source error:"))
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
			Expect(out).To(ContainSubstring("overridden"))
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

			Expect(strings.TrimSpace(out)).To(Equal("ccp filter status\n\nNo filters found."))
		})
	})
})

func validStatusFilterYAML(filterID string) string {
	return "version: 1\nfilter: " + filterID + "\ncases:\n  - id: passthrough\n    passthrough: true\n"
}
