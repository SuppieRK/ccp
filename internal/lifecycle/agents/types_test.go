package agents

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("agent type helpers", func() {
	ginkgo.DescribeTable("NormalizeToolID",
		func(input string, expected string) {
			Expect(NormalizeToolID(input)).To(Equal(expected))
		},
		ginkgo.Entry("passes through canonical ids", "roocode", "roocode"),
		ginkgo.Entry("maps aliases to their canonical id", "costrict", "roocode"),
	)

	ginkgo.Describe("SupportedTools", func() {
		ginkgo.It("returns tool ids in sorted order", func() {
			adapters := map[string]Adapter{
				"zeta":  stubAdapter{id: "zeta"},
				"alpha": stubAdapter{id: "alpha"},
				"beta":  stubAdapter{id: "beta"},
			}

			Expect(SupportedTools(adapters)).To(Equal([]string{"alpha", "beta", "zeta"}))
		})

		ginkgo.It("includes supported aliases when the canonical adapter is present", func() {
			adapters := map[string]Adapter{
				string(AgentRooCode): stubAdapter{id: string(AgentRooCode)},
				string(AgentCodex):   stubAdapter{id: string(AgentCodex)},
			}

			Expect(SupportedTools(adapters)).To(Equal([]string{"codex", "costrict", "roocode"}))
		})
	})

	ginkgo.Describe("ValidateSelectedTools", func() {
		var adapters map[string]Adapter

		ginkgo.BeforeEach(func() {
			adapters = map[string]Adapter{"alpha": stubAdapter{id: "alpha"}}
		})

		ginkgo.It("accepts supported tools", func() {
			Expect(ValidateSelectedTools([]string{"alpha"}, adapters)).To(Succeed())
		})

		ginkgo.It("rejects unsupported tools", func() {
			Expect(ValidateSelectedTools([]string{"beta"}, adapters)).To(HaveOccurred())
		})
	})

	ginkgo.Describe("DetectTools", func() {
		var (
			root string
		)

		ginkgo.BeforeEach(func() {
			root = ginkgo.GinkgoT().TempDir()
		})

		ginkgo.It("returns only detected adapter roots", func() {
			Expect(os.Mkdir(filepath.Join(root, "alpha-root"), 0o755)).To(Succeed())

			adapters := map[string]Adapter{
				"alpha": stubAdapter{id: "alpha", detectDir: "alpha-root"},
				"beta":  stubAdapter{id: "beta"},
			}

			Expect(DetectTools(root, adapters)).To(Equal([]string{"alpha"}))
		})

		ginkgo.It("ignores non-directory collisions", func() {
			filePath := filepath.Join(root, "alpha-root")
			Expect(os.WriteFile(filePath, []byte("not-a-dir"), 0o644)).To(Succeed())
			Expect(os.Mkdir(filepath.Join(root, "beta-root"), 0o755)).To(Succeed())

			adapters := map[string]Adapter{
				"alpha": stubAdapter{id: "alpha", detectDir: "alpha-root"},
				"beta":  stubAdapter{id: "beta", detectDir: "beta-root"},
			}

			Expect(DetectTools(root, adapters)).To(Equal([]string{"beta"}))
		})

		ginkgo.It("prefers explicit detector implementations when present", func() {
			adapters := map[string]Adapter{
				"alpha": NewManagedContextLinkAdapter(ManagedContextLinkAdapterSpec{
					ID: ID("alpha"),
					Detect: func(scopeRoot string) bool {
						return true
					},
				}),
			}

			Expect(DetectTools(root, adapters)).To(Equal([]string{"alpha"}))
		})
	})

	ginkgo.Describe("InstallPlannedArtifacts", func() {
		ginkgo.It("tracks applied and noop artifacts while preserving hook executability", func() {
			tmpDir := ginkgo.GinkgoT().TempDir()
			plan := []PlannedArtifact{
				{
					Kind:    ArtifactHook,
					Path:    filepath.Join(tmpDir, "hooks", "rewrite.sh"),
					Content: "#!/bin/sh\nexit 0\n",
					Perm:    0o755,
				},
				{
					Kind:    ArtifactSettings,
					Path:    filepath.Join(tmpDir, "settings.json"),
					Content: "{}\n",
					Perm:    0o644,
				},
			}

			first, err := InstallPlannedArtifacts(plan, writeFileWriter)
			Expect(err).NotTo(HaveOccurred())
			Expect(first).To(Equal(InstallResult{Applied: 2}))

			second, err := InstallPlannedArtifacts(plan, writeFileWriter)
			Expect(err).NotTo(HaveOccurred())
			Expect(second).To(Equal(InstallResult{Noop: 2}))

			info, err := os.Stat(plan[0].Path)
			Expect(err).NotTo(HaveOccurred())
			if runtime.GOOS == "windows" {
				Expect(info.Mode().IsRegular()).To(BeTrue())
			} else {
				Expect(info.Mode().Perm()).To(Equal(privateHookMode))
			}
		})

		ginkgo.It("does not make non-hook artifacts executable", func() {
			tmpDir := ginkgo.GinkgoT().TempDir()
			settingsPath := filepath.Join(tmpDir, "settings.json")
			plan := []PlannedArtifact{{
				Kind:    ArtifactSettings,
				Path:    settingsPath,
				Content: "{}\n",
				Perm:    0o644,
			}}

			res, err := InstallPlannedArtifacts(plan, writeFileWriter)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(InstallResult{Applied: 1}))

			info, err := os.Stat(settingsPath)
			Expect(err).NotTo(HaveOccurred())
			if runtime.GOOS == "windows" {
				Expect(info.Mode().IsRegular()).To(BeTrue())
			} else {
				Expect(info.Mode().Perm() & 0o111).To(BeZero())
			}
		})
	})

	ginkgo.DescribeTable("resolving scoped paths",
		func(resolve func(string, string) string, base string, rel string, expected string) {
			Expect(resolve(base, rel)).To(Equal(expected))
		},
		ginkgo.Entry("uses the home directory when present", ResolveHomeScopedPath, filepath.Join("home"), filepath.Join(".agent", "settings.json"), filepath.Join("home", ".agent", "settings.json")),
		ginkgo.Entry("falls back to the relative path when home is empty", ResolveHomeScopedPath, "", filepath.Join(".agent", "settings.json"), filepath.Join(".agent", "settings.json")),
		ginkgo.Entry("uses the repo root when present", ResolveRepoScopedPath, filepath.Join("repo"), filepath.Join(".agent", "settings.json"), filepath.Join("repo", ".agent", "settings.json")),
		ginkgo.Entry("falls back to the relative path when repo is empty", ResolveRepoScopedPath, "", filepath.Join(".agent", "settings.json"), filepath.Join(".agent", "settings.json")),
	)

	ginkgo.Describe("NewBuiltInAdapters", func() {
		ginkgo.It("contains every built-in catalog id with a matching adapter id", func() {
			adapters, err := NewBuiltInAdapters()
			Expect(err).NotTo(HaveOccurred())

			for _, spec := range BuiltInAdapterCatalog() {
				id := string(spec.ID)
				adapter, ok := adapters[id]
				Expect(ok).To(BeTrue(), "expected adapter %q", id)
				Expect(adapter.ID()).To(Equal(id))
			}
		})
	})
})
