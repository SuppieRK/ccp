package agents

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("context link families", func() {
	ginkgo.Describe("recipe content exceptions", func() {
		ginkgo.DescribeTable("renders the expected managed content shape",
			func(render func() string, mustContain []string, mustNotContain []string) {
				content := render()
				for _, needle := range mustContain {
					Expect(content).To(ContainSubstring(needle))
				}
				for _, needle := range mustNotContain {
					Expect(content).NotTo(ContainSubstring(needle))
				}
			},
			ginkgo.Entry("amazon-q", amazonQRuleContent, []string{
				"## CCP Integration (Managed)",
				ccpRawEscapeHatch,
			}, []string{"alwaysApply: true", "description:", ccpManagedBlockStart}),
			ginkgo.Entry("cline", clineRuleContent, []string{
				"## CCP Integration (Managed)",
				ccpRawEscapeHatch,
			}, []string{"alwaysApply: true", "description:", "trigger: always_on", ccpManagedBlockStart}),
			ginkgo.Entry("cursor", cursorRuleContent, []string{
				"alwaysApply: true",
				"description: Route shell commands through ccp",
			}, []string{ccpManagedBlockStart}),
			ginkgo.Entry("roocode", roocodeRuleContent, []string{
				"## CCP Integration (Managed)",
			}, []string{ccpManagedBlockStart}),
			ginkgo.Entry("trae", traeRuleContent, []string{
				"## CCP Integration (Managed)",
				ccpRawEscapeHatch,
			}, []string{"alwaysApply: true", "trigger: always_on", ccpManagedBlockStart}),
			ginkgo.Entry("windsurf", windsurfRuleContent, []string{
				"## CCP Integration (Managed)",
				ccpRawEscapeHatch,
			}, []string{"alwaysApply: true", "description:", "trigger: always_on", ccpManagedBlockStart}),
		)
	})

	ginkgo.Describe("managed context adapters", func() {
		var (
			tmpDir string
			ctx    Context
		)

		ginkgo.BeforeEach(func() {
			tmpDir = ginkgo.GinkgoT().TempDir()
			ctx = Context{ScopeRoot: filepath.Join(tmpDir, "repo"), HomeDir: filepath.Join(tmpDir, "home")}
		})

		ginkgo.It("keeps antigravity and gemini on the same target path", func() {
			antigravityPlan := NewManagedContextAdapter(antigravityContextSpec).Plan(ctx)
			geminiPlan := NewManagedContextAdapter(geminiContextSpec).Plan(ctx)

			Expect(antigravityPlan).To(HaveLen(1))
			Expect(geminiPlan).To(HaveLen(1))
			Expect(antigravityPlan[0].Path).To(Equal(geminiPlan[0].Path))
		})
	})

	ginkgo.Describe("managed context link plans", func() {
		var (
			tmpDir string
			ctx    Context
		)

		ginkgo.BeforeEach(func() {
			tmpDir = ginkgo.GinkgoT().TempDir()
			ctx = Context{ScopeRoot: filepath.Join(tmpDir, "repo"), HomeDir: filepath.Join(tmpDir, "home")}
		})

		ginkgo.DescribeTable("plans the config file and linked context file",
			func(spec ManagedContextLinkAdapterSpec, wantConfig string, wantGuide string) {
				plan := NewManagedContextLinkAdapter(spec).Plan(ctx)
				Expect(plan).To(HaveLen(2))
				Expect(plan[0].Path).To(HaveSuffix(wantConfig))
				Expect(plan[1].Path).To(HaveSuffix(wantGuide))
			},
			ginkgo.Entry("aider", aiderContextLinkSpec, ".aider.conf.yml", ".aider.rules.md"),
			ginkgo.Entry("crush", crushContextLinkSpec, filepath.Join(".config", "crush", "crush.json"), filepath.Join(".config", "crush", "CRUSH.md")),
			ginkgo.Entry("qwen", qwenContextLinkSpec, filepath.Join(".qwen", "settings.json"), filepath.Join(".qwen", "AGENTS.md")),
		)
	})

	ginkgo.Describe("managed hook settings plans", func() {
		var (
			tmpDir string
			ctx    Context
		)

		ginkgo.BeforeEach(func() {
			tmpDir = ginkgo.GinkgoT().TempDir()
			ctx = Context{ScopeRoot: filepath.Join(tmpDir, "repo"), HomeDir: filepath.Join(tmpDir, "home")}
		})

		ginkgo.DescribeTable("plans the hook script and settings file",
			func(spec ManagedHookSettingsAdapterSpec, wantRoot string, wantSettings string) {
				plan := NewManagedHookSettingsAdapter(spec).Plan(ctx)
				Expect(plan).To(HaveLen(2))
				Expect(plan[0].Path).To(ContainSubstring(wantRoot))
				Expect(plan[0].Path).To(HaveSuffix(filepath.Join("hooks", spec.HookScriptName)))
				Expect(plan[1].Path).To(ContainSubstring(wantRoot))
				Expect(plan[1].Path).To(HaveSuffix(wantSettings))
			},
			ginkgo.Entry("codebuddy", codebuddyHookSettingsSpec, ".codebuddy", "settings.json"),
		)
	})

	ginkgo.Describe("managed JS plugin plans", func() {
		var (
			tmpDir string
			ctx    Context
		)

		ginkgo.BeforeEach(func() {
			tmpDir = ginkgo.GinkgoT().TempDir()
			ctx = Context{ScopeRoot: tmpDir, HomeDir: filepath.Join(tmpDir, "home")}
		})

		ginkgo.DescribeTable("plans one plugin artifact and detects the expected root",
			func(spec ManagedJSPluginAdapterSpec) {
				adapter := NewManagedJSPluginAdapter(spec)
				Expect(adapter.Plan(ctx)).To(HaveLen(1))
				Expect(adapter.DetectRoot(tmpDir)).To(ContainSubstring(spec.DetectRootPath))
			},
			ginkgo.Entry("opencode", openCodeJSPluginSpec),
			ginkgo.Entry("kilocode", kilocodeJSPluginSpec),
		)
	})

	ginkgo.Describe("aider config helpers", func() {
		var (
			tmpDir     string
			configPath string
			readPath   string
		)

		ginkgo.BeforeEach(func() {
			tmpDir = ginkgo.GinkgoT().TempDir()
			configPath = filepath.Join(tmpDir, aiderConfigPath)
			readPath = filepath.Join(tmpDir, aiderRulesPath)
		})

		ginkgo.It("preserves unrelated read entries when adding and removing the managed path", func() {
			Expect(os.WriteFile(configPath, []byte("read:\n  - CONVENTIONS.md\nmodel: sonnet\n"), 0o644)).To(Succeed())

			updated, err := upsertAiderReadConfig(configPath, readPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated).To(ContainSubstring("- CONVENTIONS.md"))
			Expect(updated).To(ContainSubstring("- " + readPath))

			Expect(os.WriteFile(configPath, []byte(updated), 0o644)).To(Succeed())

			updated, changed, removeAll, err := removeAiderReadConfig(configPath, readPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeTrue())
			Expect(removeAll).To(BeFalse())
			Expect(updated).NotTo(ContainSubstring(readPath))
			Expect(updated).To(ContainSubstring("CONVENTIONS.md"))
			Expect(updated).To(ContainSubstring("model: sonnet"))
		})
	})

	ginkgo.Describe("crush config helpers", func() {
		var (
			tmpDir      string
			configPath  string
			contextPath string
		)

		ginkgo.BeforeEach(func() {
			tmpDir = ginkgo.GinkgoT().TempDir()
			configPath = filepath.Join(tmpDir, "crush.json")
			contextPath = filepath.Join(tmpDir, "CRUSH.md")
		})

		ginkgo.It("adds, verifies, and removes the managed context path while preserving unrelated config", func() {
			Expect(os.WriteFile(configPath, []byte("{\n  \"theme\": \"dark\",\n  \"options\": {\n    \"context_paths\": [\n      \"/tmp/team.md\"\n    ]\n  }\n}\n"), 0o644)).To(Succeed())

			updated, err := upsertCrushConfig(configPath, contextPath)
			Expect(err).NotTo(HaveOccurred())

			var root map[string]any
			Expect(json.Unmarshal([]byte(updated), &root)).To(Succeed())
			options, _ := root["options"].(map[string]any)
			Expect(options).NotTo(BeNil())
			Expect(slicesContainsPath(crushContextPaths(options["context_paths"]), contextPath)).To(BeTrue())

			Expect(os.WriteFile(configPath, []byte(updated), 0o644)).To(Succeed())

			ok, err := crushConfigUsesContext(configPath, contextPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())

			removed, changed, removeAll, err := removeCrushContextPath(configPath, contextPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeTrue())
			Expect(removeAll).To(BeFalse())
			Expect(removed).NotTo(ContainSubstring(contextPath))
			Expect(removed).To(ContainSubstring(`"theme": "dark"`))
		})

		ginkgo.It("ignores non-slice context path input", func() {
			Expect(crushContextPaths("unexpected")).To(BeEmpty())
		})
	})

	ginkgo.Describe("qwen settings helpers", func() {
		var (
			tmpDir       string
			settingsPath string
		)

		ginkgo.BeforeEach(func() {
			tmpDir = ginkgo.GinkgoT().TempDir()
			settingsPath = filepath.Join(tmpDir, "settings.json")
		})

		ginkgo.It("reads an empty settings file as an empty map", func() {
			Expect(os.WriteFile(settingsPath, nil, 0o644)).To(Succeed())
			root, err := readQwenSettings(settingsPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(root).To(BeEmpty())
		})

		ginkgo.It("adds the AGENTS.md context entry while preserving unrelated settings", func() {
			Expect(os.WriteFile(settingsPath, []byte("{\n  \"theme\": \"light\"\n}\n"), 0o644)).To(Succeed())

			updated, err := upsertQwenSettings(settingsPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated).To(ContainSubstring(`"theme": "light"`))
			Expect(updated).To(ContainSubstring(`"fileName": "AGENTS.md"`))

			Expect(os.WriteFile(settingsPath, []byte(updated), 0o644)).To(Succeed())
			ok, err := qwenSettingsUseAgents(settingsPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
		})

		ginkgo.It("removes only the managed AGENTS.md entry", func() {
			Expect(os.WriteFile(settingsPath, []byte("{\n  \"theme\": \"light\",\n  \"context\": {\n    \"fileName\": \"AGENTS.md\"\n  }\n}\n"), 0o644)).To(Succeed())

			removed, changed, removeAll, err := removeQwenSettings(settingsPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeTrue())
			Expect(removeAll).To(BeFalse())
			Expect(removed).NotTo(ContainSubstring(`"fileName": "AGENTS.md"`))
			Expect(removed).To(ContainSubstring(`"theme": "light"`))
		})
	})
})
