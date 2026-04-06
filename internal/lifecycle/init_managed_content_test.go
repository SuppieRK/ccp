package lifecycle

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newInitManagedWorkspace() lifecycleWorkspace {
	root := GinkgoT().TempDir()
	home := filepath.Join(root, "home")
	work := filepath.Join(root, "work")
	Expect(os.MkdirAll(home, 0o755)).To(Succeed())
	Expect(os.MkdirAll(work, 0o755)).To(Succeed())

	setHomeDirForSpec(home)
	withWorkingDir(work)
	return lifecycleWorkspace{root: root, home: home, work: work}
}

func expectNoBackups(path string) {
	matches, err := filepath.Glob(path + ".bak.*")
	Expect(err).NotTo(HaveOccurred())
	Expect(matches).To(BeEmpty())
}

var _ = Describe("init managed content and integrations", func() {
	Context("when managing GitHub Copilot instructions", func() {
		var (
			ws   lifecycleWorkspace
			path string
		)

		BeforeEach(func() {
			ws = newInitManagedWorkspace()
			path = filepath.Join(ws.home, initCopilotDir, initCopilotFileName)
		})

		It("writes the managed block", func() {
			Expect(RunInit([]string{initToolsFlag, "github-copilot"})).To(Succeed())

			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())

			text := string(body)
			Expect(text).To(ContainSubstring("<!-- BEGIN: CCP MANAGED BLOCK -->"))
			Expect(text).To(ContainSubstring("<!-- END: CCP MANAGED BLOCK -->"))
			Expect(text).To(ContainSubstring(initRawEscapeHatch))
		})

		It("keeps reruns stable", func() {
			Expect(RunInit([]string{initToolsFlag, "github-copilot"})).To(Succeed())

			before, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(RunInit([]string{initToolsFlag, "github-copilot"})).To(Succeed())

			after, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(after)).To(Equal(string(before)))
		})

		It("replaces only the managed region", func() {
			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			Expect(os.WriteFile(path, []byte("# User Header\n\ncustom content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nold content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"), 0o644)).To(Succeed())
			Expect(RunInit([]string{initToolsFlag, "github-copilot"})).To(Succeed())

			updated, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())

			text := string(updated)
			Expect(text).To(ContainSubstring("# User Header"))
			Expect(text).To(ContainSubstring("# Tail"))
			Expect(text).NotTo(ContainSubstring("old content"))
		})
	})

	Context("when managing Gemini instructions", func() {
		var (
			ws   lifecycleWorkspace
			path string
		)

		BeforeEach(func() {
			ws = newInitManagedWorkspace()
			path = filepath.Join(ws.home, initGeminiDir, initGeminiFileName)
		})

		It("writes the managed block", func() {
			Expect(RunInit([]string{initToolsFlag, "gemini"})).To(Succeed())

			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())

			text := string(body)
			Expect(text).To(ContainSubstring("<!-- BEGIN: CCP MANAGED BLOCK -->"))
			Expect(text).To(ContainSubstring("<!-- END: CCP MANAGED BLOCK -->"))
			Expect(text).To(ContainSubstring(initRawEscapeHatch))
		})

		It("keeps reruns stable", func() {
			Expect(RunInit([]string{initToolsFlag, "gemini"})).To(Succeed())

			before, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(RunInit([]string{initToolsFlag, "gemini"})).To(Succeed())

			after, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(after)).To(Equal(string(before)))
		})

		It("replaces only the managed region", func() {
			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			Expect(os.WriteFile(path, []byte("# User Header\n\ncustom content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nold content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"), 0o644)).To(Succeed())
			Expect(RunInit([]string{initToolsFlag, "gemini"})).To(Succeed())

			updated, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())

			text := string(updated)
			Expect(text).To(ContainSubstring("# User Header"))
			Expect(text).To(ContainSubstring("# Tail"))
			Expect(text).NotTo(ContainSubstring("old content"))
		})
	})

	Context("when managing the cursor rule", func() {
		var (
			ws   lifecycleWorkspace
			path string
		)

		BeforeEach(func() {
			ws = newInitManagedWorkspace()
			Expect(os.MkdirAll(filepath.Join(ws.work, initCursorDir), 0o755)).To(Succeed())
			path = filepath.Join(ws.work, initCursorDir, "rules", initCursorRuleName)
		})

		It("writes the expected rule content", func() {
			Expect(RunInit([]string{initToolsFlag, "cursor"})).To(Succeed())

			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())

			text := string(body)
			Expect(text).To(ContainSubstring("alwaysApply: true"))
			Expect(text).To(ContainSubstring(initRawEscapeHatch))
			Expect(text).NotTo(ContainSubstring("<!-- BEGIN: CCP MANAGED BLOCK -->"))
		})

		It("keeps reruns stable", func() {
			Expect(RunInit([]string{initToolsFlag, "cursor"})).To(Succeed())

			beforeInfo, err := os.Stat(path)
			Expect(err).NotTo(HaveOccurred())

			before, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(RunInit([]string{initToolsFlag, "cursor"})).To(Succeed())

			afterInfo, err := os.Stat(path)
			Expect(err).NotTo(HaveOccurred())

			after, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(after)).To(Equal(string(before)))
			Expect(afterInfo.ModTime()).To(Equal(beforeInfo.ModTime()))

			matches, globErr := filepath.Glob(path + ".bak.*")
			Expect(globErr).NotTo(HaveOccurred())
			Expect(matches).To(BeEmpty())
		})
	})

	DescribeTable("when managing repo-scoped plain rule files",
		func(tool, markerPath, relPath string) {
			ws := newInitManagedWorkspace()
			Expect(os.MkdirAll(filepath.Join(ws.work, markerPath), 0o755)).To(Succeed())

			Expect(RunInit([]string{initToolsFlag, tool})).To(Succeed())

			body, err := os.ReadFile(filepath.Join(ws.work, relPath))
			Expect(err).NotTo(HaveOccurred())

			text := string(body)
			Expect(text).To(ContainSubstring("## CCP Integration (Managed)"))
			Expect(text).To(ContainSubstring(initRawEscapeHatch))
			Expect(text).NotTo(ContainSubstring("<!-- BEGIN: CCP MANAGED BLOCK -->"))
		},
		Entry("cline", "cline", initClineDir, filepath.Join(initClineDir, initClineRuleName)),
		Entry("windsurf", "windsurf", initWindsurfDir, filepath.Join(initWindsurfDir, "rules", initWindsurfRuleName)),
	)

	It("preserves unrelated CodeBuddy settings", func() {
		ws := newInitManagedWorkspace()
		Expect(os.MkdirAll(filepath.Join(ws.work, initCodeBuddyDir), 0o755)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(ws.home, initCodeBuddyDir), 0o755)).To(Succeed())

		settingsPath := filepath.Join(ws.home, initCodeBuddyDir, initSettingsFileName)
		Expect(os.WriteFile(settingsPath, []byte("{\n  \"theme\": \"light\"\n}\n"), 0o644)).To(Succeed())

		Expect(RunInit([]string{initToolsFlag, "codebuddy"})).To(Succeed())

		body, err := os.ReadFile(settingsPath)
		Expect(err).NotTo(HaveOccurred())

		text := string(body)
		Expect(text).To(ContainSubstring(`"theme": "light"`))
		Expect(text).To(ContainSubstring(`"PreToolUse"`))

		escapedHook := strings.ReplaceAll(filepath.Join(ws.home, initCodeBuddyDir, "hooks", initRewriteScriptName), "\\", "\\\\")
		Expect(strings.Count(text, escapedHook)).To(Equal(1))
	})

	It("preserves user content and config for Crush", func() {
		ws := newInitManagedWorkspace()
		Expect(os.MkdirAll(filepath.Join(ws.work, initCrushDir), 0o755)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(ws.home, ".config", "crush"), 0o755)).To(Succeed())

		agentsPath := filepath.Join(ws.home, ".config", "crush", "CRUSH.md")
		Expect(os.WriteFile(agentsPath, []byte("# User Header\n\ncustom content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nold content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"), 0o644)).To(Succeed())

		configPath := filepath.Join(ws.home, ".config", "crush", "crush.json")
		Expect(os.WriteFile(configPath, []byte("{\n  \"theme\": \"dark\"\n}\n"), 0o644)).To(Succeed())

		Expect(RunInit([]string{initToolsFlag, "crush"})).To(Succeed())

		updated, err := os.ReadFile(agentsPath)
		Expect(err).NotTo(HaveOccurred())

		text := string(updated)
		Expect(text).To(ContainSubstring("# User Header"))
		Expect(text).To(ContainSubstring("# Tail"))
		Expect(text).NotTo(ContainSubstring("old content"))

		cfg, err := os.ReadFile(configPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(cfg)).To(ContainSubstring(`"theme": "dark"`))
	})

	Context("when installing claude artifacts", func() {
		var (
			ws           lifecycleWorkspace
			hookPath     string
			settingsPath string
			awareness    string
			guide        string
		)

		BeforeEach(func() {
			ws = newInitManagedWorkspace()
			hookPath = filepath.Join(ws.home, initClaudeDir, "hooks", initRewriteScriptName)
			settingsPath = filepath.Join(ws.home, initClaudeDir, initSettingsFileName)
			awareness = filepath.Join(ws.home, initClaudeDir, "CCP.md")
			guide = filepath.Join(ws.home, initClaudeDir, "CLAUDE.md")
		})

		It("installs the expected home targets", func() {
			Expect(RunInit([]string{initToolsFlag, "claude"})).To(Succeed())
			for _, path := range []string{hookPath, settingsPath, awareness, guide} {
				_, err := os.Stat(path)
				Expect(err).NotTo(HaveOccurred(), path)
			}
		})

		It("writes valid hook and settings content", func() {
			Expect(RunInit([]string{initToolsFlag, "claude"})).To(Succeed())
			if runtime.GOOS != "windows" {
				info, err := os.Stat(hookPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(info.Mode() & 0o111).NotTo(BeZero())

				cmd := exec.Command("bash", "-n", hookPath)
				out, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred(), string(out))
			}

			settings, err := os.ReadFile(settingsPath)
			Expect(err).NotTo(HaveOccurred())

			settingsText := string(settings)
			Expect(settingsText).To(ContainSubstring(`"PreToolUse"`))
			Expect(settingsText).To(ContainSubstring(`"matcher": "Bash"`))
			Expect(settingsText).To(ContainSubstring(`"command": "` + strings.ReplaceAll(hookPath, "\\", "\\\\") + `"`))

			guideBody, err := os.ReadFile(guide)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(guideBody)).To(ContainSubstring("@CCP.md"))
		})

		It("preserves unrelated Claude settings", func() {
			Expect(os.MkdirAll(filepath.Dir(settingsPath), 0o755)).To(Succeed())
			Expect(os.WriteFile(settingsPath, []byte("{\n  \"theme\": \"light\",\n  \"editor\": {\n    \"vimMode\": true\n  }\n}\n"), 0o644)).To(Succeed())

			Expect(RunInit([]string{initToolsFlag, "claude"})).To(Succeed())

			settings, err := os.ReadFile(settingsPath)
			Expect(err).NotTo(HaveOccurred())

			settingsText := string(settings)
			Expect(settingsText).To(ContainSubstring(`"theme": "light"`))
			Expect(settingsText).To(ContainSubstring(`"vimMode": true`))
			Expect(settingsText).To(ContainSubstring(`"PreToolUse"`))

			escapedHook := strings.ReplaceAll(hookPath, "\\", "\\\\")
			Expect(strings.Count(settingsText, escapedHook)).To(Equal(1))
		})

		It("fails without overwriting invalid Claude settings", func() {
			Expect(os.MkdirAll(filepath.Dir(settingsPath), 0o755)).To(Succeed())
			original := []byte("{invalid\n")
			Expect(os.WriteFile(settingsPath, original, 0o644)).To(Succeed())

			err := RunInit([]string{initToolsFlag, "claude"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid claude settings file"))

			current, readErr := os.ReadFile(settingsPath)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(current).To(Equal(original))
		})

		It("keeps managed artifacts stable on rerun", func() {
			Expect(RunInit([]string{initToolsFlag, "claude"})).To(Succeed())

			paths := []string{hookPath, settingsPath, awareness, guide}
			beforeInfo := map[string]os.FileInfo{}
			beforeData := map[string][]byte{}
			for _, path := range paths {
				info, err := os.Stat(path)
				Expect(err).NotTo(HaveOccurred())
				data, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())
				beforeInfo[path] = info
				beforeData[path] = data
			}

			Expect(RunInit([]string{initToolsFlag, "claude"})).To(Succeed())

			for _, path := range paths {
				info, err := os.Stat(path)
				Expect(err).NotTo(HaveOccurred())
				data, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())
				Expect(info.ModTime()).To(Equal(beforeInfo[path].ModTime()))
				Expect(string(data)).To(Equal(string(beforeData[path])))
				matches, globErr := filepath.Glob(path + ".bak.*")
				Expect(globErr).NotTo(HaveOccurred())
				Expect(matches).To(BeEmpty())
			}
		})
	})
})
