package agents

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("adapter catalog", func() {
	var (
		tmpDir string
		ctx    Context
	)

	ginkgo.BeforeEach(func() {
		tmpDir = ginkgo.GinkgoT().TempDir()
		ctx = Context{ScopeRoot: filepath.Join(tmpDir, "repo"), HomeDir: filepath.Join(tmpDir, "home")}
		Expect(os.MkdirAll(ctx.ScopeRoot, 0o755)).To(Succeed())
		Expect(os.MkdirAll(ctx.HomeDir, 0o755)).To(Succeed())
	})

	ginkgo.Describe("NewBuiltInAdapters", func() {
		ginkgo.It("includes every built-in adapter from the catalog", func() {
			adapters, err := NewBuiltInAdapters()
			Expect(err).NotTo(HaveOccurred())

			for _, spec := range BuiltInAdapterCatalog() {
				id := string(spec.ID)
				Expect(adapters).To(HaveKey(id))
			}
		})
	})

	ginkgo.Describe("adaptersFromCatalog", func() {
		ginkgo.It("rejects duplicate canonical ids", func() {
			_, err := adaptersFromCatalog([]BuiltInAdapterSpec{
				{ID: AgentCodex, New: func() Adapter { return NewManagedContextAdapter(codexContextSpec) }},
				{ID: AgentCodex, New: func() Adapter { return NewManagedContextAdapter(codexContextSpec) }},
			})

			Expect(err).To(HaveOccurred())
		})

		ginkgo.It("rejects nil adapters returned by the catalog constructor", func() {
			_, err := adaptersFromCatalog([]BuiltInAdapterSpec{
				{ID: AgentCodex, New: func() Adapter { return nil }},
			})

			Expect(err).To(MatchError("nil adapter for codex"))
		})

		ginkgo.It("rejects adapters whose runtime id does not match the catalog id", func() {
			_, err := adaptersFromCatalog([]BuiltInAdapterSpec{
				{ID: AgentCodex, New: func() Adapter { return ClaudeAdapter{} }},
			})

			Expect(err).To(HaveOccurred())
		})
	})

	ginkgo.Describe("built-in context-link specs", func() {
		ginkgo.DescribeTable("detects aider config files only when a real file exists",
			func(setup func(string) error, want bool) {
				Expect(setup(tmpDir)).To(Succeed())
				Expect(aiderContextLinkSpec.Detect(tmpDir)).To(Equal(want))
			},
			ginkgo.Entry("missing config", func(string) error { return nil }, false),
			ginkgo.Entry("config file present", func(root string) error {
				return os.WriteFile(filepath.Join(root, aiderConfigPath), []byte("read: []\n"), 0o644)
			}, true),
			ginkgo.Entry("config path is a directory", func(root string) error {
				return os.MkdirAll(filepath.Join(root, aiderConfigPath), 0o755)
			}, false),
		)

		ginkgo.DescribeTable("surfaces context-link verification errors",
			func(verify func(string, Context) error, setup func(string) error, want func(string) string) {
				configPath := filepath.Join(tmpDir, "config")
				Expect(setup(configPath)).To(Succeed())

				err := verify(configPath, ctx)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(want(configPath)))
			},
			ginkgo.Entry("for crush invalid config content", crushContextLinkSpec.VerifyConfig, func(configPath string) error {
				return os.WriteFile(configPath, []byte("{"), 0o644)
			}, func(configPath string) string { return fmt.Sprintf(crushConfigErrFmt, configPath) }),
		)
	})

	ginkgo.Describe("built-in hook-setting specs", func() {
		ginkgo.DescribeTable("surfaces settings verification errors",
			func(setup func(string) error, want func(string) string) {
				settingsPath := filepath.Join(tmpDir, "settings.json")
				hookPath := filepath.Join(tmpDir, "hooks", codebuddyHookScriptName)
				Expect(os.MkdirAll(filepath.Dir(hookPath), 0o755)).To(Succeed())
				Expect(setup(settingsPath)).To(Succeed())

				err := codebuddyHookSettingsSpec.VerifySettings(settingsPath, hookPath)
				Expect(err).To(MatchError(want(settingsPath)))
			},
			ginkgo.Entry("for invalid settings content", func(settingsPath string) error {
				return os.WriteFile(settingsPath, []byte("{"), 0o644)
			}, func(settingsPath string) string {
				return fmt.Sprintf("invalid codebuddy settings file: %s", settingsPath)
			}),
		)

		ginkgo.DescribeTable("shapes uninstall results from settings cleanup",
			func(setup func(string, string) error, want InstallResult, wantErr bool) {
				settingsPath := filepath.Join(tmpDir, "settings.json")
				hookPath := filepath.Join(tmpDir, "hooks", codebuddyHookScriptName)
				Expect(os.MkdirAll(filepath.Dir(hookPath), 0o755)).To(Succeed())
				Expect(setup(settingsPath, hookPath)).To(Succeed())

				res, err := codebuddyHookSettingsSpec.UninstallSettings(settingsPath, hookPath)
				if wantErr {
					Expect(err).To(HaveOccurred())
					Expect(res).To(Equal(InstallResult{}))
					return
				}

				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(want))
			},
			ginkgo.Entry("reports an applied change when the managed hook is removed", func(settingsPath, hookPath string) error {
				content := preToolUseCommandSettingsContent(hookPath)
				return os.WriteFile(settingsPath, []byte(content), 0o644)
			}, InstallResult{Applied: 1}, false),
			ginkgo.Entry("reports a noop when the managed hook is absent", func(settingsPath, hookPath string) error {
				content := preToolUseCommandSettingsContent(filepath.Join(filepath.Dir(hookPath), "other-hook.sh"))
				return os.WriteFile(settingsPath, []byte(content), 0o644)
			}, InstallResult{Noop: 1}, false),
			ginkgo.Entry("propagates cleanup errors from the settings helper", func(settingsPath, hookPath string) error {
				return os.MkdirAll(settingsPath, 0o755)
			}, InstallResult{}, true),
		)
	})
})
