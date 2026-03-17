package lifecycle

import (
	"bytes"
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("filter new", func() {
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
