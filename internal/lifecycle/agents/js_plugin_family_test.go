package agents

import (
	"os"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("OpenCode JS plugin family", func() {
	var (
		tmpDir  string
		home    string
		ctx     Context
		adapter Adapter
	)

	ginkgo.BeforeEach(func() {
		tmpDir = ginkgo.GinkgoT().TempDir()
		home = filepath.Join(tmpDir, "home")
		ctx = Context{ScopeRoot: tmpDir, HomeDir: home}
		adapter = NewManagedJSPluginAdapter(openCodeJSPluginSpec)
	})

	ginkgo.Describe("adapter lifecycle", func() {
		ginkgo.It("installs, verifies, and uninstalls the managed plugin", func() {
			Expect(adapter.ID()).To(Equal("opencode"))

			_, err := adapter.Install(ctx, writeFileWriter)
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter.Verify(ctx)).To(Succeed())

			res, err := adapter.(interface {
				Uninstall(Context) (InstallResult, error)
			}).Uninstall(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.Applied).To(Equal(1))
		})
	})

	ginkgo.Describe("verification and root resolution", func() {
		var (
			configRoot string
			pluginPath string
		)

		ginkgo.BeforeEach(func() {
			ctx = Context{ScopeRoot: home, HomeDir: home}
			configRoot = managedJSPluginConfigRoot(ctx, openCodeJSPluginSpec.ConfigDirName)
			pluginPath = filepath.Join(configRoot, "plugins", managedJSPluginFileName)
		})

		ginkgo.It("uses the global config root for opencode", func() {
			Expect(configRoot).To(ContainSubstring(filepath.Join(".config", "opencode")))
		})

		ginkgo.It("fails verification for invalid plugin content", func() {
			Expect(os.MkdirAll(filepath.Dir(pluginPath), 0o755)).To(Succeed())
			Expect(os.WriteFile(pluginPath, []byte("export default {}"), 0o644)).To(Succeed())

			Expect(NewManagedJSPluginAdapter(openCodeJSPluginSpec).Verify(ctx)).To(HaveOccurred())
		})
	})

	ginkgo.Describe("plugin content", func() {
		ginkgo.It("keeps ordinary quoted commands in the normal rewrite path", func() {
			script := managedBashRewritePluginContent()
			Expect(script).To(ContainSubstring(`if (/\$\(|\$\{|<</.test(command))`))
			Expect(script).NotTo(ContainSubstring(`if (/['"\\]|\$\(|\$\{|<</.test(command))`))
			Expect(script).To(ContainSubstring(`command.replace(/(^|\|\||&&|\||;)\s*(?!ccp\b)/g, "$1 ccp ")`))
			Expect(script).To(ContainSubstring(`if (rewritten === command)`))
			Expect(script).To(ContainSubstring(`output.args.command = rewritten;`))
		})
	})
})
