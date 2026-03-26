package agents

import (
	"os"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Family conformance", func() {
	var (
		tmpDir string
		ctx    Context
	)

	ginkgo.BeforeEach(func() {
		tmpDir = ginkgo.GinkgoT().TempDir()
		ctx = Context{ScopeRoot: filepath.Join(tmpDir, "repo"), HomeDir: filepath.Join(tmpDir, "home")}
		Expect(os.MkdirAll(ctx.HomeDir, 0o755)).To(Succeed())
	})

	ginkgo.Describe("managed context adapters", func() {
		ginkgo.DescribeTable("conforms to the context family contract",
			func(spec ManagedContextFileAdapterSpec) {
				Expect(os.MkdirAll(ResolveRepoScopedPath(ctx.ScopeRoot, spec.DetectRootPath), 0o755)).To(Succeed())

				adapter := NewManagedContextAdapter(spec)
				Expect(adapter.ID()).To(Equal(string(spec.ID)))
				Expect(adapter.DetectRoot(ctx.ScopeRoot)).To(HaveSuffix(spec.DetectRootPath))

				plan := adapter.Plan(ctx)
				Expect(plan).To(HaveLen(1))
				switch spec.TargetScope {
				case managedContextTargetRepo:
					Expect(plan[0].Path).To(HavePrefix(ctx.ScopeRoot))
				default:
					Expect(plan[0].Path).To(HavePrefix(ctx.HomeDir))
				}

				_, err := adapter.Install(ctx, writeFileWriter)
				Expect(err).NotTo(HaveOccurred())
				Expect(adapter.Verify(ctx)).To(Succeed())

				res, err := adapter.Uninstall(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.Applied).To(Equal(1))
			},
			ginkgo.Entry("antigravity", antigravityContextSpec),
			ginkgo.Entry("auggie", auggieContextSpec),
			ginkgo.Entry("codex", codexContextSpec),
			ginkgo.Entry("factory", factoryContextSpec),
			ginkgo.Entry("gemini", geminiContextSpec),
			ginkgo.Entry("github-copilot", githubCopilotContextSpec),
			ginkgo.Entry("kiro", kiroContextSpec),
			ginkgo.Entry("pi", piContextSpec),
			ginkgo.Entry("qoder", qoderContextSpec),
		)
	})

	ginkgo.Describe("managed rule file adapters", func() {
		ginkgo.DescribeTable("conforms to the rule file family contract",
			func(spec managedRuleFileAdapterSpec) {
				Expect(os.MkdirAll(expectedRuleDetectRoot(ctx.ScopeRoot, spec), 0o755)).To(Succeed())

				adapter := newManagedRuleFileAdapterFromSpec(spec)
				Expect(adapter.ID()).To(Equal(string(spec.ID)))
				Expect(adapter.DetectRoot(ctx.ScopeRoot)).To(Equal(expectedRuleDetectRoot(ctx.ScopeRoot, spec)))

				plan := adapter.Plan(ctx)
				Expect(plan).To(HaveLen(1))
				Expect(plan[0].Path).To(Equal(expectedRuleTarget(ctx, spec)))

				_, err := adapter.Install(ctx, writeFileWriter)
				Expect(err).NotTo(HaveOccurred())
				Expect(adapter.Verify(ctx)).To(Succeed())

				res, err := adapter.(Uninstaller).Uninstall(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.Applied).To(Equal(1))
			},
			ginkgo.Entry("amazon-q", managedRuleFileAdapterSpecs[0]),
			ginkgo.Entry("cline", managedRuleFileAdapterSpecs[1]),
			ginkgo.Entry("cursor", managedRuleFileAdapterSpecs[2]),
			ginkgo.Entry("roocode", managedRuleFileAdapterSpecs[3]),
			ginkgo.Entry("trae", managedRuleFileAdapterSpecs[4]),
			ginkgo.Entry("windsurf", managedRuleFileAdapterSpecs[5]),
		)
	})

	ginkgo.Describe("managed JS plugin adapters", func() {
		ginkgo.DescribeTable("conforms to the JS plugin family contract",
			func(spec ManagedJSPluginAdapterSpec) {
				Expect(os.MkdirAll(ResolveRepoScopedPath(ctx.ScopeRoot, spec.DetectRootPath), 0o755)).To(Succeed())

				adapter := NewManagedJSPluginAdapter(spec)
				Expect(adapter.ID()).To(Equal(string(spec.ID)))

				_, err := adapter.Install(ctx, writeFileWriter)
				Expect(err).NotTo(HaveOccurred())
				Expect(adapter.Verify(ctx)).To(Succeed())

				res, err := adapter.Uninstall(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.Applied).To(Equal(1))
			},
			ginkgo.Entry("opencode", openCodeJSPluginSpec),
			ginkgo.Entry("kilocode", kilocodeJSPluginSpec),
		)
	})

	ginkgo.Describe("managed context link adapters", func() {
		ginkgo.DescribeTable("conforms to the context link family contract",
			func(spec ManagedContextLinkAdapterSpec) {
				Expect(os.MkdirAll(ResolveRepoScopedPath(ctx.ScopeRoot, spec.DetectRootPath), 0o755)).To(Succeed())

				adapter := NewManagedContextLinkAdapter(spec)
				Expect(adapter.ID()).To(Equal(string(spec.ID)))
				Expect(adapter.DetectRoot(ctx.ScopeRoot)).To(Equal(filepath.Join(ctx.ScopeRoot, spec.DetectRootPath)))

				_, err := adapter.Install(ctx, writeFileWriter)
				Expect(err).NotTo(HaveOccurred())
				Expect(adapter.Verify(ctx)).To(Succeed())

				res, err := adapter.Uninstall(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.Applied).To(BeNumerically(">", 0))
			},
			ginkgo.Entry("aider", aiderContextLinkSpec),
			ginkgo.Entry("crush", crushContextLinkSpec),
			ginkgo.Entry("qwen", qwenContextLinkSpec),
		)

		ginkgo.DescribeTable("fails verification when linked context files go missing",
			func(spec ManagedContextLinkAdapterSpec) {
				Expect(os.MkdirAll(ResolveRepoScopedPath(ctx.ScopeRoot, spec.DetectRootPath), 0o755)).To(Succeed())

				adapter := NewManagedContextLinkAdapter(spec)
				_, err := adapter.Install(ctx, writeFileWriter)
				Expect(err).NotTo(HaveOccurred())

				contextPlan := NewManagedContextAdapter(spec.ContextSpec).Plan(ctx)
				Expect(contextPlan).To(HaveLen(1))
				Expect(os.Remove(contextPlan[0].Path)).To(Succeed())

				Expect(adapter.Verify(ctx)).To(HaveOccurred())
			},
			ginkgo.Entry("aider", aiderContextLinkSpec),
			ginkgo.Entry("crush", crushContextLinkSpec),
			ginkgo.Entry("qwen", qwenContextLinkSpec),
		)
	})

	ginkgo.Describe("managed hook settings adapters", func() {
		ginkgo.DescribeTable("conforms to the hook settings family contract",
			func(spec ManagedHookSettingsAdapterSpec) {
				Expect(os.MkdirAll(ResolveRepoScopedPath(ctx.ScopeRoot, spec.DetectRootPath), 0o755)).To(Succeed())

				adapter := NewManagedHookSettingsAdapter(spec)
				Expect(adapter.ID()).To(Equal(string(spec.ID)))
				Expect(adapter.DetectRoot(ctx.ScopeRoot)).To(Equal(filepath.Join(ctx.ScopeRoot, spec.DetectRootPath)))

				_, err := adapter.Install(ctx, writeFileWriter)
				Expect(err).NotTo(HaveOccurred())
				Expect(adapter.Verify(ctx)).To(Succeed())

				res, err := adapter.Uninstall(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.Applied).To(BeNumerically(">", 0))
			},
			ginkgo.Entry("codebuddy", codebuddyHookSettingsSpec),
		)

		ginkgo.It("returns settings write failures after installing the hook artifact", func() {
			adapter := NewManagedHookSettingsAdapter(ManagedHookSettingsAdapterSpec{
				ID:             ID("broken-hook"),
				DetectRootPath: ".broken-agent",
				Root: func(ctx Context) string {
					return filepath.Join(ctx.ScopeRoot, ".broken-agent")
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
				VerifySettings: nil,
				UninstallSettings: func(settingsPath, hookPath string) (InstallResult, error) {
					return InstallResult{}, nil
				},
				MissingHookFmt:     "missing hook: %s",
				MissingSettingsFmt: "missing settings: %s",
			})

			calls := 0
			_, err := adapter.Install(ctx, func(path string, data []byte, perm os.FileMode) (bool, error) {
				calls++
				if calls == 2 {
					return false, os.ErrPermission
				}
				return writeFileWriter(path, data, perm)
			})

			Expect(err).To(MatchError(os.ErrPermission))
			Expect(adapter.hookPath(ctx)).To(BeAnExistingFile())
			Expect(adapter.Verify(ctx)).To(HaveOccurred())
		})

		ginkgo.It("skips custom settings verification when none is configured", func() {
			adapter := NewManagedHookSettingsAdapter(ManagedHookSettingsAdapterSpec{
				ID:             ID("verify-nil"),
				DetectRootPath: ".verify-nil",
				Root: func(ctx Context) string {
					return filepath.Join(ctx.ScopeRoot, ".verify-nil")
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
				VerifySettings: nil,
				UninstallSettings: func(settingsPath, hookPath string) (InstallResult, error) {
					return InstallResult{}, nil
				},
				MissingHookFmt:     "missing hook: %s",
				MissingSettingsFmt: "missing settings: %s",
			})

			_, err := adapter.Install(ctx, writeFileWriter)
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter.Verify(ctx)).To(Succeed())
		})
	})
})
