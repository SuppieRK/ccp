package lifecycle

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type additionalDetectionCase struct {
	tool         string
	markerPath   string
	artifactRoot string
	artifactRel  string
}

type additionalManagedFileCase struct {
	tool       string
	markerPath string
	targetRoot string
	targetRel  string
}

var _ = Describe("init additional integration coverage", func() {
	var detectionWS initDetectionWorkspace

	BeforeEach(func() {
		detectionWS = newInitDetectionWorkspace()
	})

	DescribeTable("detects additional tools from repository markers",
		func(tc additionalDetectionCase) {
			Expect(os.MkdirAll(filepath.Join(detectionWS.root, tc.markerPath), 0o755)).To(Succeed())
			Expect(RunInit(nil)).To(Succeed())

			base := detectionWS.home
			if tc.artifactRoot == "root" {
				base = detectionWS.root
			}
			_, err := os.Stat(filepath.Join(base, tc.artifactRel))
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("pi", additionalDetectionCase{
			tool:         "pi",
			markerPath:   initPiDir,
			artifactRoot: "root",
			artifactRel:  initPiAppendSystemRel,
		}),
		Entry("roocode", additionalDetectionCase{
			tool:        "roocode",
			markerPath:  initRooCodeDir,
			artifactRel: filepath.Join(initRooCodeDir, "rules", initRooCodeRuleName),
		}),
		Entry("trae", additionalDetectionCase{
			tool:         "trae",
			markerPath:   initTraeDir,
			artifactRoot: "root",
			artifactRel:  filepath.Join(initTraeDir, "rules", initTraeRuleName),
		}),
		Entry("windsurf", additionalDetectionCase{
			tool:         "windsurf",
			markerPath:   initWindsurfDir,
			artifactRoot: "root",
			artifactRel:  filepath.Join(initWindsurfDir, "rules", initWindsurfRuleName),
		}),
	)

	It("maps the costrict alias to roocode", func() {
		Expect(os.MkdirAll(filepath.Join(detectionWS.root, initRooCodeDir), 0o755)).To(Succeed())

		Expect(RunInit([]string{initToolsFlag, "costrict"})).To(Succeed())

		_, err := os.Stat(filepath.Join(detectionWS.home, initRooCodeDir, "rules", initRooCodeRuleName))
		Expect(err).NotTo(HaveOccurred())
	})

	DescribeTable("writes managed content for additional file-backed integrations",
		func(tc additionalManagedFileCase) {
			ws := newInitManagedWorkspace()
			Expect(os.MkdirAll(filepath.Join(ws.work, tc.markerPath), 0o755)).To(Succeed())

			Expect(RunInit([]string{initToolsFlag, tc.tool})).To(Succeed())

			base := ws.home
			if tc.targetRoot == "work" {
				base = ws.work
			}
			body, err := os.ReadFile(filepath.Join(base, tc.targetRel))
			Expect(err).NotTo(HaveOccurred())

			text := string(body)
			Expect(text).To(ContainSubstring("<!-- BEGIN: CCP MANAGED BLOCK -->"))
			Expect(text).To(ContainSubstring("<!-- END: CCP MANAGED BLOCK -->"))
			Expect(text).To(ContainSubstring(initRawEscapeHatch))
		},
		Entry("qoder", additionalManagedFileCase{
			tool:       "qoder",
			markerPath: initQoderDir,
			targetRel:  filepath.Join(initQoderDir, initAgentsFileName),
		}),
		Entry("factory", additionalManagedFileCase{
			tool:       "factory",
			markerPath: initFactoryDir,
			targetRel:  filepath.Join(initFactoryDir, initAgentsFileName),
		}),
		Entry("auggie", additionalManagedFileCase{
			tool:       "auggie",
			markerPath: initAuggieDir,
			targetRoot: "work",
			targetRel:  initAgentsFileName,
		}),
		Entry("pi", additionalManagedFileCase{
			tool:       "pi",
			markerPath: initPiDir,
			targetRoot: "work",
			targetRel:  initPiAppendSystemRel,
		}),
	)

	It("does not create root AGENTS.md for the pi integration", func() {
		ws := newInitManagedWorkspace()
		Expect(os.MkdirAll(filepath.Join(ws.work, initPiDir), 0o755)).To(Succeed())

		Expect(RunInit([]string{initToolsFlag, "pi"})).To(Succeed())

		_, err := os.Stat(filepath.Join(ws.work, initAgentsFileName))
		Expect(err).To(MatchError(os.ErrNotExist))
		_, err = os.Stat(filepath.Join(ws.work, initPiAppendSystemRel))
		Expect(err).NotTo(HaveOccurred())
	})

	DescribeTable("replaces only the managed region for additional file-backed integrations",
		func(tc additionalManagedFileCase) {
			ws := newInitManagedWorkspace()
			Expect(os.MkdirAll(filepath.Join(ws.work, tc.markerPath), 0o755)).To(Succeed())

			base := ws.home
			if tc.targetRoot == "work" {
				base = ws.work
			}
			targetPath := filepath.Join(base, tc.targetRel)
			Expect(os.MkdirAll(filepath.Dir(targetPath), 0o755)).To(Succeed())
			Expect(os.WriteFile(targetPath, []byte("# User Header\n\ncustom content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nold content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"), 0o644)).To(Succeed())

			Expect(RunInit([]string{initToolsFlag, tc.tool})).To(Succeed())

			updated, err := os.ReadFile(targetPath)
			Expect(err).NotTo(HaveOccurred())

			text := string(updated)
			Expect(text).To(ContainSubstring("# User Header"))
			Expect(text).To(ContainSubstring("# Tail"))
			Expect(text).NotTo(ContainSubstring("old content"))
		},
		Entry("qoder", additionalManagedFileCase{
			tool:       "qoder",
			markerPath: initQoderDir,
			targetRel:  filepath.Join(initQoderDir, initAgentsFileName),
		}),
		Entry("factory", additionalManagedFileCase{
			tool:       "factory",
			markerPath: initFactoryDir,
			targetRel:  filepath.Join(initFactoryDir, initAgentsFileName),
		}),
		Entry("auggie", additionalManagedFileCase{
			tool:       "auggie",
			markerPath: initAuggieDir,
			targetRoot: "work",
			targetRel:  initAgentsFileName,
		}),
		Entry("pi", additionalManagedFileCase{
			tool:       "pi",
			markerPath: initPiDir,
			targetRoot: "work",
			targetRel:  initPiAppendSystemRel,
		}),
	)

	It("writes antigravity to the gemini-family home target and keeps it stable on rerun", func() {
		ws := newInitManagedWorkspace()
		Expect(os.MkdirAll(filepath.Join(ws.work, initAntigravityDir), 0o755)).To(Succeed())

		Expect(RunInit([]string{initToolsFlag, "antigravity"})).To(Succeed())

		path := filepath.Join(ws.home, initGeminiDir, initGeminiFileName)
		body, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())

		text := string(body)
		Expect(text).To(ContainSubstring("<!-- BEGIN: CCP MANAGED BLOCK -->"))
		Expect(text).To(ContainSubstring("<!-- END: CCP MANAGED BLOCK -->"))
		Expect(text).To(ContainSubstring(initRawEscapeHatch))

		before := text
		Expect(RunInit([]string{initToolsFlag, "antigravity"})).To(Succeed())

		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(after)).To(Equal(before))
		Expect(strings.Count(string(after), "<!-- BEGIN: CCP MANAGED BLOCK -->")).To(Equal(1))
		Expect(strings.Count(string(after), "<!-- END: CCP MANAGED BLOCK -->")).To(Equal(1))
	})

	It("writes the kilocode plugin and keeps reruns idempotent", func() {
		ws := newInitManagedWorkspace()
		Expect(os.MkdirAll(filepath.Join(ws.work, initKilocodeDir), 0o755)).To(Succeed())

		Expect(RunInit([]string{initToolsFlag, "kilocode"})).To(Succeed())

		path := filepath.Join(ws.home, ".config", "kilocode", "plugins", initOpenCodeRewriteJS)
		body, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())

		text := string(body)
		Expect(text).To(ContainSubstring(`"tool.execute.before"`))
		Expect(text).To(ContainSubstring(`trimmed.startsWith("ccp ")`))

		beforeInfo, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		before := text

		Expect(RunInit([]string{initToolsFlag, "kilocode"})).To(Succeed())

		afterInfo, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())

		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(after)).To(Equal(before))
		Expect(afterInfo.ModTime()).To(Equal(beforeInfo.ModTime()))
		expectNoBackups(path)
	})

	It("keeps qwen managed output stable and preserves unrelated settings", func() {
		ws := newInitManagedWorkspace()
		Expect(os.MkdirAll(filepath.Join(ws.work, initQwenDir), 0o755)).To(Succeed())

		settingsPath := filepath.Join(ws.home, initQwenDir, initSettingsFileName)
		Expect(os.MkdirAll(filepath.Dir(settingsPath), 0o755)).To(Succeed())
		Expect(os.WriteFile(settingsPath, []byte("{\n  \"theme\": \"dark\"\n}\n"), 0o644)).To(Succeed())

		Expect(RunInit([]string{initToolsFlag, "qwen"})).To(Succeed())

		agentsPath := filepath.Join(ws.home, initQwenDir, initAgentsFileName)
		agentsBody, err := os.ReadFile(agentsPath)
		Expect(err).NotTo(HaveOccurred())

		agentsText := string(agentsBody)
		Expect(agentsText).To(ContainSubstring("<!-- BEGIN: CCP MANAGED BLOCK -->"))
		Expect(agentsText).To(ContainSubstring("<!-- END: CCP MANAGED BLOCK -->"))
		Expect(agentsText).To(ContainSubstring(initRawEscapeHatch))

		settingsBody, err := os.ReadFile(settingsPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(settingsBody)).To(ContainSubstring(`"theme": "dark"`))
		Expect(string(settingsBody)).To(ContainSubstring(`"fileName": "AGENTS.md"`))

		before := agentsText
		Expect(RunInit([]string{initToolsFlag, "qwen"})).To(Succeed())

		after, err := os.ReadFile(agentsPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(after)).To(Equal(before))
		Expect(strings.Count(string(after), "<!-- BEGIN: CCP MANAGED BLOCK -->")).To(Equal(1))
		Expect(strings.Count(string(after), "<!-- END: CCP MANAGED BLOCK -->")).To(Equal(1))
	})

	It("replaces only the managed region for qwen agents", func() {
		ws := newInitManagedWorkspace()
		Expect(os.MkdirAll(filepath.Join(ws.work, initQwenDir), 0o755)).To(Succeed())

		path := filepath.Join(ws.home, initQwenDir, initAgentsFileName)
		Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
		Expect(os.WriteFile(path, []byte("# User Header\n\ncustom content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nold content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"), 0o644)).To(Succeed())
		Expect(RunInit([]string{initToolsFlag, "qwen"})).To(Succeed())

		updated, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())

		text := string(updated)
		Expect(text).To(ContainSubstring("# User Header"))
		Expect(text).To(ContainSubstring("# Tail"))
		Expect(text).NotTo(ContainSubstring("old content"))
	})

})
