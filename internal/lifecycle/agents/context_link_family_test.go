package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

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
			ginkgo.Entry("amazon-q", ccpManagedGuidanceMarkdown, []string{
				"## CCP Integration (Managed)",
				ccpRawEscapeHatch,
			}, []string{"alwaysApply: true", "description:", ccpManagedBlockStart}),
			ginkgo.Entry("cline", ccpManagedGuidanceMarkdown, []string{
				"## CCP Integration (Managed)",
				ccpRawEscapeHatch,
			}, []string{"alwaysApply: true", "description:", "trigger: always_on", ccpManagedBlockStart}),
			ginkgo.Entry("cursor", cursorRuleContent, []string{
				"alwaysApply: true",
				"description: Route shell commands through ccp",
			}, []string{ccpManagedBlockStart}),
			ginkgo.Entry("roocode", ccpManagedGuidanceMarkdown, []string{
				"## CCP Integration (Managed)",
			}, []string{ccpManagedBlockStart}),
			ginkgo.Entry("trae", ccpManagedGuidanceMarkdown, []string{
				"## CCP Integration (Managed)",
				ccpRawEscapeHatch,
			}, []string{"alwaysApply: true", "trigger: always_on", ccpManagedBlockStart}),
			ginkgo.Entry("windsurf", ccpManagedGuidanceMarkdown, []string{
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

		ginkgo.It("detects the managed root with the default filesystem check", func() {
			adapter := NewManagedContextLinkAdapter(ManagedContextLinkAdapterSpec{
				ID:             ID("detectable"),
				DetectRootPath: ".detectable",
			})

			Expect(adapter.Detect(ctx.ScopeRoot)).To(BeFalse())
			Expect(os.MkdirAll(filepath.Join(ctx.ScopeRoot, ".detectable"), 0o755)).To(Succeed())
			Expect(adapter.Detect(ctx.ScopeRoot)).To(BeTrue())
		})

		ginkgo.It("installs, verifies, and uninstalls the config and linked context files", func() {
			adapter := NewManagedContextLinkAdapter(ManagedContextLinkAdapterSpec{
				ID:             ID("linked"),
				DetectRootPath: ".linked",
				ContextSpec: ManagedContextFileAdapterSpec{
					ID:            ID("linked"),
					TargetRelPath: filepath.Join(".linked", "AGENTS.md"),
					TargetScope:   managedContextTargetRepo,
					MissingFmt:    "missing linked agents file: %s",
					MarkersFmt:    "missing linked managed markers in %s",
				},
				ConfigPath: func(ctx Context) string {
					return filepath.Join(ctx.ScopeRoot, ".linked", "settings.json")
				},
				ConfigPlanContent: func(ctx Context) string {
					return "{\n  \"context\": \"AGENTS.md\"\n}\n"
				},
				UpsertConfig: func(configPath string, ctx Context) (string, error) {
					return "{\n  \"context\": \"AGENTS.md\"\n}\n", nil
				},
				VerifyConfig: func(configPath string, ctx Context) error {
					raw, err := os.ReadFile(configPath)
					if err != nil {
						return err
					}
					Expect(string(raw)).To(ContainSubstring(`"context": "AGENTS.md"`))
					return nil
				},
				RemoveConfig: func(configPath string, ctx Context) (updated string, changed bool, removeAll bool, err error) {
					return "", true, true, nil
				},
			})

			installRes, err := adapter.Install(ctx, writeFileWriter)
			Expect(err).NotTo(HaveOccurred())
			Expect(installRes.Applied).To(Equal(2))
			Expect(adapter.Verify(ctx)).To(Succeed())

			uninstallRes, err := adapter.Uninstall(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(uninstallRes.Applied).To(Equal(2))
			_, err = os.Stat(filepath.Join(ctx.ScopeRoot, ".linked", "settings.json"))
			Expect(err).To(MatchError(os.ErrNotExist))
			_, err = os.Stat(filepath.Join(ctx.ScopeRoot, ".linked", "AGENTS.md"))
			Expect(err).To(MatchError(os.ErrNotExist))
		})
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

	ginkgo.Describe("managed hook settings install and verify", func() {
		var (
			tmpDir  string
			ctx     Context
			adapter ManagedHookSettingsAdapter
		)

		ginkgo.BeforeEach(func() {
			tmpDir = ginkgo.GinkgoT().TempDir()
			ctx = Context{ScopeRoot: filepath.Join(tmpDir, "repo"), HomeDir: filepath.Join(tmpDir, "home")}
			adapter = NewManagedHookSettingsAdapter(ManagedHookSettingsAdapterSpec{
				ID:             ID("test-hook"),
				DetectRootPath: ".test-agent",
				Root: func(ctx Context) string {
					return filepath.Join(ctx.ScopeRoot, ".test-agent")
				},
				HookScriptName: "rewrite.sh",
				SettingsName:   "settings.json",
				HookContent: func() string {
					return "#!/bin/sh\nexit 0\n"
				},
				PlanSettingsContent: func(hookPath string) string {
					return "{\n  \"hook\": \"" + hookPath + "\"\n}\n"
				},
				UpsertSettings: func(settingsPath, hookPath string) (string, error) {
					return "{\n  \"hook\": \"" + hookPath + "\"\n}\n", nil
				},
				VerifySettings: func(settingsPath, hookPath string) error {
					return nil
				},
				UninstallSettings: func(settingsPath, hookPath string) (InstallResult, error) {
					return InstallResult{}, nil
				},
				MissingHookFmt:     "missing hook: %s",
				MissingSettingsFmt: "missing settings: %s",
			})
		})

		ginkgo.It("reasserts executable permissions when reinstalling an unchanged hook", func() {
			if runtime.GOOS == "windows" {
				ginkgo.Skip("executable bit checks are skipped on Windows")
			}

			_, err := adapter.Install(ctx, writeFileWriter)
			Expect(err).NotTo(HaveOccurred())

			hookPath := adapter.hookPath(ctx)
			Expect(os.Chmod(hookPath, 0o644)).To(Succeed())

			_, err = adapter.Install(ctx, writeFileWriter)
			Expect(err).NotTo(HaveOccurred())

			info, err := os.Stat(hookPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm() & 0o111).NotTo(BeZero())
		})

		ginkgo.It("fails verify when the managed hook script is not executable", func() {
			if runtime.GOOS == "windows" {
				ginkgo.Skip("executable bit checks are skipped on Windows")
			}

			_, err := adapter.Install(ctx, writeFileWriter)
			Expect(err).NotTo(HaveOccurred())

			hookPath := adapter.hookPath(ctx)
			Expect(os.Chmod(hookPath, 0o644)).To(Succeed())

			Expect(adapter.Verify(ctx)).To(HaveOccurred())
		})
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

		ginkgo.DescribeTable("normalizes aider read entries",
			func(input any, expected []string) {
				Expect(normalizeAiderRead(input)).To(Equal(expected))
			},
			ginkgo.Entry("nil becomes nil", nil, []string(nil)),
			ginkgo.Entry("an empty string is ignored", "", []string(nil)),
			ginkgo.Entry("a single string becomes one entry", "RULES.md", []string{"RULES.md"}),
			ginkgo.Entry("a string slice drops empty entries", []string{"RULES.md", "", "TEAM.md"}, []string{"RULES.md", "TEAM.md"}),
			ginkgo.Entry("an any slice keeps only non-empty strings", []any{"RULES.md", 7, "", "TEAM.md"}, []string{"RULES.md", "TEAM.md"}),
			ginkgo.Entry("unsupported types are ignored", 42, []string(nil)),
		)
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

		ginkgo.DescribeTable("removes managed crush context paths with the expected cleanup scope",
			func(setup func() string, expectedChanged bool, expectedRemoveAll bool) {
				expectedUpdated := setup()

				updated, changed, removeAll, err := removeCrushContextPath(configPath, contextPath)

				Expect(err).NotTo(HaveOccurred())
				Expect(changed).To(Equal(expectedChanged))
				Expect(removeAll).To(Equal(expectedRemoveAll))
				Expect(updated).To(Equal(expectedUpdated))
			},
			ginkgo.Entry("drops only the matching path and preserves other options", func() string {
				root := map[string]any{
					"options": map[string]any{
						"context_paths": []any{contextPath, "/tmp/team.md"},
						"theme":         "dark",
					},
				}
				raw, err := json.MarshalIndent(root, "", "  ")
				Expect(err).NotTo(HaveOccurred())
				Expect(os.WriteFile(configPath, append(raw, '\n'), 0o644)).To(Succeed())

				root = map[string]any{
					"options": map[string]any{
						"context_paths": []string{"/tmp/team.md"},
						"theme":         "dark",
					},
				}
				raw, err = json.MarshalIndent(root, "", "  ")
				Expect(err).NotTo(HaveOccurred())
				return string(append(raw, '\n'))
			},
				true,
				false,
			),
			ginkgo.Entry("removes the entire file when the managed path is the last option", func() string {
				root := map[string]any{
					"options": map[string]any{
						"context_paths": []any{contextPath},
					},
				}
				raw, err := json.MarshalIndent(root, "", "  ")
				Expect(err).NotTo(HaveOccurred())
				Expect(os.WriteFile(configPath, append(raw, '\n'), 0o644)).To(Succeed())
				return ""
			},
				true,
				true,
			),
			ginkgo.Entry("does nothing when the managed path is absent", func() string {
				Expect(os.WriteFile(configPath, []byte("{\n  \"options\": {\n    \"context_paths\": [\n      \"/tmp/team.md\"\n    ]\n  }\n}\n"), 0o644)).To(Succeed())
				return ""
			},
				false,
				false,
			),
		)

		ginkgo.DescribeTable("normalizes crush context path inputs",
			func(input any, expected []string) {
				Expect(crushContextPaths(input)).To(Equal(expected))
			},
			ginkgo.Entry("treats nil as empty", nil, []string(nil)),
			ginkgo.Entry("treats blank scalar input as empty", "   ", []string(nil)),
			ginkgo.Entry("keeps only non-empty string entries", []any{"one", "", 7, "two"}, []string{"one", "two"}),
		)
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

		ginkgo.DescribeTable("removes qwen settings with the expected cleanup scope",
			func(initial string, expectedUpdated string, expectedChanged bool, expectedRemoveAll bool) {
				Expect(os.WriteFile(settingsPath, []byte(initial), 0o644)).To(Succeed())

				updated, changed, removeAll, err := removeQwenSettings(settingsPath)

				Expect(err).NotTo(HaveOccurred())
				Expect(changed).To(Equal(expectedChanged))
				Expect(removeAll).To(Equal(expectedRemoveAll))
				Expect(updated).To(Equal(expectedUpdated))
			},
			ginkgo.Entry("drops only the managed fileName entry when context has other keys",
				"{\n  \"theme\": \"light\",\n  \"context\": {\n    \"fileName\": \"AGENTS.md\",\n    \"mode\": \"repo\"\n  }\n}\n",
				"{\n  \"context\": {\n    \"mode\": \"repo\"\n  },\n  \"theme\": \"light\"\n}\n",
				true,
				false,
			),
			ginkgo.Entry("removes the whole file when AGENTS.md is the only setting",
				"{\n  \"context\": {\n    \"fileName\": \"AGENTS.md\"\n  }\n}\n",
				"",
				true,
				true,
			),
			ginkgo.Entry("does nothing when the context uses a different file",
				"{\n  \"context\": {\n    \"fileName\": \"RULES.md\"\n  }\n}\n",
				"",
				false,
				false,
			),
		)
	})
})
