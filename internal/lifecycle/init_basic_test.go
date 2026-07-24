package lifecycle

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/SuppieRK/cmdshape/internal/lifecycle/agents"
	"github.com/SuppieRK/cmdshape/internal/workspaces"
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
			_, err := os.Stat(filepath.Join(ws.home, ".config", "cmdshape", "filters"))
			Expect(err).To(MatchError(os.ErrNotExist))
		})

		It("does not persist an init manifest", func() {
			_, err := os.Stat(filepath.Join(ws.home, ".config", "cmdshape", initConfigFileName))
			Expect(err).To(MatchError(os.ErrNotExist))
		})

		It("registers the current workspace even before metrics exist", func() {
			entries, err := workspaces.ListPath(workspaces.PathForHome(ws.home))
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(resolvedPath(entries[0].CWD)).To(Equal(resolvedPath(ws.work)))
			Expect(entries[0].MetricsPath).To(BeEmpty())
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
			_, err := os.Stat(filepath.Join(ws.home, ".config", "cmdshape", initConfigFileName))
			Expect(err).To(MatchError(os.ErrNotExist))
		})

		It("allows subsequent init runs for additional tools without a manifest", func() {
			Expect(RunInit([]string{initToolsFlag, "opencode"})).To(Succeed())
			_, err := os.Stat(filepath.Join(ws.home, ".config", "opencode", "plugins", initOpenCodeRewriteJS))
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates the workspace registry for noop reruns too", func() {
			Expect(RunInit([]string{initToolsFlag, "cursor"})).To(Succeed())
			entries, err := workspaces.ListPath(workspaces.PathForHome(ws.home))
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(resolvedPath(entries[0].CWD)).To(Equal(resolvedPath(ws.work)))
		})
	})

	It("keeps init successful when the workspace registry cannot be updated", func() {
		registryPath := workspaces.PathForHome(ws.home)
		Expect(os.MkdirAll(registryPath, 0o755)).To(Succeed())

		stderr, err := captureStderrOutput(func() error {
			return RunInit(args)
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(stderr).To(ContainSubstring("cmdshape init: warning: could not update workspace registry"))
		Expect(filepath.Join(ws.home, ".config", "opencode", "plugins", initOpenCodeRewriteJS)).To(BeAnExistingFile())
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
			Expect(content).To(ContainSubstring(`if (command === "cmdshape") return segment;`))
		})

		It("keeps conservative fallback handling for complex commands", func() {
			body, err := os.ReadFile(pluginPath)
			Expect(err).NotTo(HaveOccurred())

			content := string(body)
			Expect(content).To(ContainSubstring(`function rewriteCommand(input)`))
			Expect(content).To(ContainSubstring(`shellBuiltinsAndKeywords`))
			Expect(content).To(ContainSubstring(`command === "find"`))
			Expect(content).To(ContainSubstring(`if (char === "|" || char === "&") return null;`))
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

	It("reports a helpful error when no tools can be auto-detected", func() {
		ws = newInitSpecWorkspace()

		_, err := resolveInitTools("", map[string]agents.Adapter{"fake": &fakeInstallAdapter{}})

		Expect(err).To(MatchError(ContainSubstring("no tools detected; specify --tools (fake)")))
	})
})

var _ = Describe("init helper functions", func() {
	DescribeTable("detecting all-noop tool states",
		func(states []toolState, want bool) {
			Expect(allToolStatesNoop(states)).To(Equal(want))
		},
		Entry("treats an empty state list as noop", nil, true),
		Entry("treats only noop states as noop", []toolState{{Tool: "codex", Status: "noop"}}, true),
		Entry("treats mixed states as non-noop", []toolState{{Tool: "codex", Status: "noop"}, {Tool: "cursor", Status: "applied"}}, false),
	)

	It("writes managed bytes once and becomes noop for unchanged content", func() {
		path := filepath.Join(GinkgoT().TempDir(), "managed", "AGENTS.md")

		changed, err := writeManagedBytes(path, []byte("managed\n"), 0o644)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())

		changed, err = writeManagedBytes(path, []byte("managed\n"), 0o644)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())
	})

	It("returns read errors from non-file targets", func() {
		path := filepath.Join(GinkgoT().TempDir(), "managed")
		Expect(os.MkdirAll(path, 0o755)).To(Succeed())

		changed, err := writeManagedBytes(path, []byte("managed\n"), 0o644)
		Expect(err).To(HaveOccurred())
		Expect(changed).To(BeFalse())
	})

	It("refuses to rewrite symlinked managed targets", func() {
		tmpDir := GinkgoT().TempDir()
		outsideDir := filepath.Join(tmpDir, "outside")
		Expect(os.MkdirAll(outsideDir, 0o755)).To(Succeed())
		outsideFile := filepath.Join(outsideDir, "AGENTS.md")
		Expect(os.WriteFile(outsideFile, []byte("keep me\n"), 0o644)).To(Succeed())

		linkDir := filepath.Join(tmpDir, ".agent")
		if err := os.Symlink(outsideDir, linkDir); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		changed, err := writeManagedBytes(filepath.Join(linkDir, "AGENTS.md"), []byte("overwrite\n"), 0o644)
		Expect(err).To(HaveOccurred())
		Expect(changed).To(BeFalse())

		body, readErr := os.ReadFile(outsideFile)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("keep me\n"))
	})
})
