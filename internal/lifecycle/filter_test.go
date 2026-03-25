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
