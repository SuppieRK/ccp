package lifecycle

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type initDetectionWorkspace struct {
	root string
	home string
}

type initDetectionCase struct {
	name         string
	markerPath   string
	expectedTool string
	artifactRoot string
	artifactRel  string
}

func newInitDetectionWorkspace() initDetectionWorkspace {
	root := GinkgoT().TempDir()
	home := filepath.Join(root, "home")
	Expect(os.MkdirAll(home, 0o755)).To(Succeed())

	setHomeDirForSpec(home)
	withWorkingDir(root)
	return initDetectionWorkspace{root: root, home: home}
}

var _ = Describe("init auto-detection", func() {
	var ws initDetectionWorkspace

	BeforeEach(func() {
		ws = newInitDetectionWorkspace()
	})

	DescribeTable("detects a tool from repository markers",
		func(tc initDetectionCase) {
			Expect(os.MkdirAll(filepath.Join(ws.root, tc.markerPath), 0o755)).To(Succeed())
			Expect(RunInit(nil)).To(Succeed())

			artifactBase := ws.home
			if tc.artifactRoot == "root" {
				artifactBase = ws.root
			}
			_, err := os.Stat(filepath.Join(artifactBase, tc.artifactRel))
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("github-copilot", initDetectionCase{
			name:         "github-copilot",
			markerPath:   ".github",
			expectedTool: "github-copilot",
			artifactRel:  filepath.Join(initCopilotDir, initCopilotFileName),
		}),
		Entry("cursor", initDetectionCase{
			name:         "cursor",
			markerPath:   initCursorDir,
			expectedTool: "cursor",
			artifactRoot: "root",
			artifactRel:  filepath.Join(initCursorDir, "rules", initCursorRuleName),
		}),
		Entry("cline", initDetectionCase{
			name:         "cline",
			markerPath:   initClineDir,
			expectedTool: "cline",
			artifactRoot: "root",
			artifactRel:  filepath.Join(initClineDir, initClineRuleName),
		}),
		Entry("gemini", initDetectionCase{
			name:         "gemini",
			markerPath:   initGeminiDir,
			expectedTool: "gemini",
			artifactRel:  filepath.Join(initGeminiDir, initGeminiFileName),
		}),
		Entry("amazon-q", initDetectionCase{
			name:         "amazon-q",
			markerPath:   initAmazonQDir,
			expectedTool: "amazon-q",
			artifactRoot: "root",
			artifactRel:  filepath.Join(initAmazonQDir, "rules", initAmazonQRuleName),
		}),
		Entry("antigravity", initDetectionCase{
			name:         "antigravity",
			markerPath:   initAntigravityDir,
			expectedTool: "antigravity",
			artifactRel:  filepath.Join(initGeminiDir, initGeminiFileName),
		}),
		Entry("kiro", initDetectionCase{
			name:         "kiro",
			markerPath:   initKiroDir,
			expectedTool: "kiro",
			artifactRel:  filepath.Join(initKiroDir, "steering", initKiroRuleName),
		}),
		Entry("kilocode", initDetectionCase{
			name:         "kilocode",
			markerPath:   initKilocodeDir,
			expectedTool: "kilocode",
			artifactRel:  filepath.Join(".config", "kilocode", "plugins", initOpenCodeRewriteJS),
		}),
		Entry("qoder", initDetectionCase{
			name:         "qoder",
			markerPath:   initQoderDir,
			expectedTool: "qoder",
			artifactRel:  filepath.Join(".qoder", "AGENTS.md"),
		}),
		Entry("factory", initDetectionCase{
			name:         "factory",
			markerPath:   initFactoryDir,
			expectedTool: "factory",
			artifactRel:  filepath.Join(initFactoryDir, initAgentsFileName),
		}),
		Entry("auggie", initDetectionCase{
			name:         "auggie",
			markerPath:   initAuggieDir,
			expectedTool: "auggie",
			artifactRoot: "root",
			artifactRel:  initAgentsFileName,
		}),
		Entry("codebuddy", initDetectionCase{
			name:         "codebuddy",
			markerPath:   initCodeBuddyDir,
			expectedTool: "codebuddy",
			artifactRel:  filepath.Join(initCodeBuddyDir, "hooks", initRewriteScriptName),
		}),
		Entry("crush", initDetectionCase{
			name:         "crush",
			markerPath:   initCrushDir,
			expectedTool: "crush",
			artifactRel:  filepath.Join(".config", "crush", "CRUSH.md"),
		}),
		Entry("windsurf", initDetectionCase{
			name:         "windsurf",
			markerPath:   initWindsurfDir,
			expectedTool: "windsurf",
			artifactRoot: "root",
			artifactRel:  filepath.Join(initWindsurfDir, "rules", initWindsurfRuleName),
		}),
	)

	It("detects aider from repository config and preserves the repo file", func() {
		Expect(os.WriteFile(filepath.Join(ws.root, initAiderConfigName), []byte("model: sonnet\n"), 0o644)).To(Succeed())

		Expect(RunInit(nil)).To(Succeed())

		configBytes, err := os.ReadFile(filepath.Join(ws.home, initAiderConfigName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(configBytes)).To(ContainSubstring(filepath.Join(ws.home, ".aider.rules.md")))

		rulesBytes, err := os.ReadFile(filepath.Join(ws.home, ".aider.rules.md"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(rulesBytes)).To(ContainSubstring("Use `ccp` as the command prefix for every executable in shell commands"))

		repoBytes, err := os.ReadFile(filepath.Join(ws.root, initAiderConfigName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(repoBytes)).To(Equal("model: sonnet\n"))
	})
})

var _ = Describe("init family-specific managed output", func() {
	var ws initDetectionWorkspace

	BeforeEach(func() {
		ws = newInitDetectionWorkspace()
	})

	It("uses home AGENTS and qwen settings for qwen detection", func() {
		Expect(os.MkdirAll(filepath.Join(ws.root, initQwenDir), 0o755)).To(Succeed())

		Expect(RunInit(nil)).To(Succeed())

		_, agentsErr := os.Stat(filepath.Join(ws.home, ".qwen", "AGENTS.md"))
		Expect(agentsErr).NotTo(HaveOccurred())

		settingsBytes, err := os.ReadFile(filepath.Join(ws.home, ".qwen", "settings.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(settingsBytes)).To(ContainSubstring(`"fileName": "AGENTS.md"`))
	})
})
