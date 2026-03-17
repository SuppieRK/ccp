package lifecycle

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go-command-compression-proxy/internal/lifecycle/agents"
)

func newInitSpecWorkspace() lifecycleWorkspace {
	root := GinkgoT().TempDir()
	home := filepath.Join(root, "home")
	Expect(os.MkdirAll(home, 0o755)).To(Succeed())

	setHomeDirForSpec(home)
	withWorkingDir(root)
	return lifecycleWorkspace{root: root, home: home, work: root}
}

var _ = Describe("init basic behavior", func() {
	var (
		ws   lifecycleWorkspace
		args []string
	)

	BeforeEach(func() {
		ws = newLifecycleWorkspaceSpec()
		args = []string{initToolsFlag, "opencode"}
	})

	Context("when managing the global init config", func() {
		BeforeEach(func() {
			args = []string{initToolsFlag, "cursor,opencode"}
			Expect(RunInit(args)).To(Succeed())
		})

		It("does not rewrite the managed home filter directory", func() {
			_, err := os.Stat(filepath.Join(ws.home, ".config", "ccp", "filters"))
			Expect(err).To(MatchError(os.ErrNotExist))
		})

		It("does not persist an init manifest", func() {
			_, err := os.Stat(filepath.Join(ws.home, ".config", "ccp", initConfigFileName))
			Expect(err).To(MatchError(os.ErrNotExist))
		})

		It("keeps reruns idempotent without writing backup artifacts", func() {
			path := filepath.Join(ws.home, ".config", "opencode", "plugins", initOpenCodeRewriteJS)

			Expect(RunInit([]string{initToolsFlag, "opencode,cursor"})).To(Succeed())
			Expect(RunInit([]string{initToolsFlag, "opencode"})).To(Succeed())

			matches, err := filepath.Glob(path + ".bak.*")
			Expect(err).NotTo(HaveOccurred())
			Expect(matches).To(BeEmpty())
		})
	})

	Context("when the repository has a .gitignore", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(filepath.Join(ws.work, initGitignoreName), []byte("node_modules\n"), 0o644)).To(Succeed())
			Expect(RunInit(args)).To(Succeed())
		})

		It("does not modify the file", func() {
			body, err := os.ReadFile(filepath.Join(ws.work, initGitignoreName))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal("node_modules\n"))
		})
	})

	Context("when the repository has no .gitignore", func() {
		BeforeEach(func() {
			Expect(RunInit(args)).To(Succeed())
		})

		It("leaves it absent", func() {
			_, err := os.Stat(filepath.Join(ws.work, initGitignoreName))
			Expect(os.IsNotExist(err)).To(BeTrue())
		})
	})

	Context("when persisting init state", func() {
		BeforeEach(func() {
			args = []string{initToolsFlag, "cursor"}
			Expect(RunInit(args)).To(Succeed())
		})

		It("leaves no init manifest behind", func() {
			_, err := os.Stat(filepath.Join(ws.home, ".config", "ccp", initConfigFileName))
			Expect(err).To(MatchError(os.ErrNotExist))
		})

		It("allows subsequent init runs for additional tools without a manifest", func() {
			Expect(RunInit([]string{initToolsFlag, "opencode"})).To(Succeed())
			_, err := os.Stat(filepath.Join(ws.home, ".config", "opencode", "plugins", initOpenCodeRewriteJS))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when repository detection would find codex", func() {
		BeforeEach(func() {
			Expect(os.MkdirAll(filepath.Join(ws.root, initCodexDir), 0o755)).To(Succeed())
			args = []string{initToolsFlag, "opencode"}
			Expect(RunInit(args)).To(Succeed())
		})

		It("installs the explicitly requested tool", func() {
			_, err := os.Stat(filepath.Join(ws.home, ".config", "opencode", "plugins", initOpenCodeRewriteJS))
			Expect(err).NotTo(HaveOccurred())
		})

		It("does not install the detected tool", func() {
			_, err := os.Stat(filepath.Join(ws.home, initCodexDir, initAgentsFileName))
			Expect(os.IsNotExist(err)).To(BeTrue())
		})
	})

	It("routes applyAdapters through the installer contract", func() {
		fake := &fakeInstallAdapter{}
		scope := agents.Context{ScopeRoot: GinkgoT().TempDir(), HomeDir: GinkgoT().TempDir()}

		states, err := applyAdapters(scope, []string{"fake"}, map[string]agents.Adapter{"fake": fake})

		Expect(err).NotTo(HaveOccurred())
		Expect(fake.installed).To(Equal(1))
		Expect(states).To(HaveLen(1))
		Expect(states[0].Status).To(Equal("applied"))
	})

	Context("when installing the opencode plugin", func() {
		var pluginPath string

		BeforeEach(func() {
			ws = newInitSpecWorkspace()
			pluginPath = filepath.Join(ws.home, ".config", "opencode", "plugins", initOpenCodeRewriteJS)
			Expect(RunInit([]string{initToolsFlag, "opencode"})).To(Succeed())
		})

		It("writes the plugin file", func() {
			_, err := os.Stat(pluginPath)
			Expect(err).NotTo(HaveOccurred())
		})

		It("registers the bash execute hook", func() {
			body, err := os.ReadFile(pluginPath)
			Expect(err).NotTo(HaveOccurred())

			content := string(body)
			Expect(content).To(ContainSubstring(`"tool.execute.before"`))
			Expect(content).To(ContainSubstring(`input.tool !== "bash"`))
		})

		It("guards already-prefixed commands", func() {
			body, err := os.ReadFile(pluginPath)
			Expect(err).NotTo(HaveOccurred())

			content := string(body)
			Expect(content).To(ContainSubstring(`trimmed.startsWith("ccp ")`))
			Expect(content).To(ContainSubstring(`trimmed === "ccp"`))
		})

		It("keeps conservative fallback handling for complex commands", func() {
			body, err := os.ReadFile(pluginPath)
			Expect(err).NotTo(HaveOccurred())

			content := string(body)
			Expect(content).To(ContainSubstring(`if (/['"\\]|\$\(|\$\{|<</.test(command))`))
			Expect(content).To(ContainSubstring(`command.replace(/(^|\|\||&&|\||;)\s*(?!ccp\b)/g, "$1 ccp ")`))
			Expect(content).To(ContainSubstring(`output.args.command = rewritten;`))
		})

		It("keeps reruns idempotent", func() {
			beforeInfo, err := os.Stat(pluginPath)
			Expect(err).NotTo(HaveOccurred())

			beforeData, err := os.ReadFile(pluginPath)
			Expect(err).NotTo(HaveOccurred())

			Expect(RunInit([]string{initToolsFlag, "opencode"})).To(Succeed())

			afterInfo, err := os.Stat(pluginPath)
			Expect(err).NotTo(HaveOccurred())

			afterData, err := os.ReadFile(pluginPath)
			Expect(err).NotTo(HaveOccurred())

			Expect(afterInfo.ModTime()).To(Equal(beforeInfo.ModTime()))
			Expect(string(afterData)).To(Equal(string(beforeData)))

			matches, globErr := filepath.Glob(pluginPath + ".bak.*")
			Expect(globErr).NotTo(HaveOccurred())
			Expect(matches).To(BeEmpty())
		})
	})

	Context("when tools are omitted and codex is detectable", func() {
		BeforeEach(func() {
			ws = newInitSpecWorkspace()
			Expect(os.MkdirAll(filepath.Join(ws.root, initCodexDir), 0o755)).To(Succeed())
			Expect(RunInit(nil)).To(Succeed())
		})

		It("installs the codex agents file", func() {
			_, err := os.Stat(filepath.Join(ws.home, initCodexDir, initAgentsFileName))
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
