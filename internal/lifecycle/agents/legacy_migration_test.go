package agents

import (
	"os"
	"path/filepath"
	"strings"

	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("legacy integration migration", func() {
	ginkgo.It("detects and removes an owned legacy rule after retaining the replacement", func() {
		root := ginkgo.GinkgoT().TempDir()
		ctx := Context{ScopeRoot: root, HomeDir: filepath.Join(root, "home")}
		legacyPath := filepath.Join(root, ".clinerules", "ccp.md")
		currentPath := filepath.Join(root, ".clinerules", "cmdshape.md")
		Expect(os.MkdirAll(filepath.Dir(legacyPath), 0o755)).To(Succeed())
		Expect(os.WriteFile(legacyPath, []byte(
			"## CCP Integration (Managed)\n\n"+legacyRuleSignature+"\n",
		), 0o644)).To(Succeed())
		Expect(os.WriteFile(currentPath, []byte(cmdshapeManagedGuidanceMarkdown()), 0o644)).To(Succeed())

		Expect(HasLegacyArtifacts(ctx, string(AgentCline))).To(BeTrue())
		Expect(CleanupLegacyArtifacts(ctx, []string{string(AgentCline)})).To(Succeed())

		Expect(legacyPath).NotTo(BeAnExistingFile())
		Expect(currentPath).To(BeAnExistingFile())
		Expect(HasLegacyArtifacts(ctx, string(AgentCline))).To(BeFalse())
	})

	ginkgo.It("keeps lookalike user files that do not have the managed signature", func() {
		root := ginkgo.GinkgoT().TempDir()
		ctx := Context{ScopeRoot: root, HomeDir: filepath.Join(root, "home")}
		legacyPath := filepath.Join(root, ".cursor", "rules", "ccp.mdc")
		Expect(os.MkdirAll(filepath.Dir(legacyPath), 0o755)).To(Succeed())
		Expect(os.WriteFile(legacyPath, []byte("# personal CCP notes\n"), 0o644)).To(Succeed())

		Expect(CleanupLegacyArtifacts(ctx, []string{string(AgentCursor)})).To(Succeed())

		Expect(legacyPath).To(BeAnExistingFile())
	})

	ginkgo.It("removes only the old hook contribution from shared settings", func() {
		home := ginkgo.GinkgoT().TempDir()
		ctx := Context{ScopeRoot: home, HomeDir: home}
		root := codebuddyRoot(ctx)
		legacyHook := filepath.Join(root, "hooks", "ccp-rewrite.sh")
		currentHook := filepath.Join(root, "hooks", codebuddyHookScriptName)
		settingsPath := filepath.Join(root, codebuddySettingsName)
		Expect(os.MkdirAll(filepath.Dir(legacyHook), 0o755)).To(Succeed())
		Expect(os.WriteFile(legacyHook, []byte(legacyHookSignature+"codebuddy\n"), 0o755)).To(Succeed())
		Expect(os.WriteFile(currentHook, []byte("# current\n"), 0o755)).To(Succeed())
		settings := preToolUseCommandSettingsContent(legacyHook)
		Expect(os.WriteFile(settingsPath, []byte(settings), 0o644)).To(Succeed())
		updated, err := upsertPreToolUseCommandSettings(settingsPath, currentHook, "invalid: %s")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(settingsPath, []byte(updated), 0o644)).To(Succeed())

		Expect(CleanupLegacyArtifacts(ctx, []string{string(AgentCodeBuddy)})).To(Succeed())

		raw, err := os.ReadFile(settingsPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(string(raw))).NotTo(BeEmpty())
		usesLegacyHook, err := codebuddySettingsUseHook(settingsPath, legacyHook)
		Expect(err).NotTo(HaveOccurred())
		Expect(usesLegacyHook).To(BeFalse())
		usesCurrentHook, err := codebuddySettingsUseHook(settingsPath, currentHook)
		Expect(err).NotTo(HaveOccurred())
		Expect(usesCurrentHook).To(BeTrue())
		Expect(legacyHook).NotTo(BeAnExistingFile())
	})

	ginkgo.It("cleans managed artifacts for retired integrations", func() {
		home := ginkgo.GinkgoT().TempDir()
		ctx := Context{ScopeRoot: home, HomeDir: home}
		continueRoot := filepath.Join(home, ".continue")
		continueHook := filepath.Join(continueRoot, "hooks", "ccp-rewrite.sh")
		continueSettings := filepath.Join(continueRoot, "settings.json")
		Expect(os.MkdirAll(filepath.Dir(continueHook), 0o755)).To(Succeed())
		Expect(os.WriteFile(continueHook, []byte(legacyHookSignature+"continue\n"), 0o755)).To(Succeed())
		Expect(os.WriteFile(
			continueSettings,
			[]byte(preToolUseCommandSettingsContent(continueHook)),
			0o644,
		)).To(Succeed())

		iflowPath := filepath.Join(home, ".iflow", "IFLOW.md")
		Expect(os.MkdirAll(filepath.Dir(iflowPath), 0o755)).To(Succeed())
		Expect(os.WriteFile(iflowPath, []byte(
			"team notes\n\n"+legacyManagedBlockStart+"\nmanaged\n"+legacyManagedBlockEnd+"\n",
		), 0o644)).To(Succeed())

		Expect(CleanupRetiredLegacyArtifacts(ctx)).To(Succeed())

		Expect(continueHook).NotTo(BeAnExistingFile())
		Expect(continueSettings).NotTo(BeAnExistingFile())
		iflow, err := os.ReadFile(iflowPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(iflow)).To(Equal("team notes\n"))
	})
})
