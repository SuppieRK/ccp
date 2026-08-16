package agents

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("ClaudeAdapter", func() {
	var (
		tmpDir  string
		home    string
		ctx     Context
		adapter Adapter
	)

	ginkgo.BeforeEach(func() {
		tmpDir = ginkgo.GinkgoT().TempDir()
		home = filepath.Join(tmpDir, "home")
		ctx = Context{ScopeRoot: filepath.Join(tmpDir, "repo"), HomeDir: home}
		Expect(os.MkdirAll(home, 0o755)).To(Succeed())
		adapter = ClaudeAdapter{}
	})

	ginkgo.It("plans, installs, verifies, and uninstalls its managed artifacts", func() {
		Expect(adapter.ID()).To(Equal("claude"))
		Expect(adapter.DetectRoot(ctx.ScopeRoot)).To(ContainSubstring(".claude"))
		Expect(adapter.Plan(ctx)).To(HaveLen(4))

		_, err := adapter.Install(ctx, writeFileWriter)
		Expect(err).NotTo(HaveOccurred())
		Expect(adapter.Verify(ctx)).To(Succeed())

		guidePath := filepath.Join(home, ".claude", claudeGuideName)
		guide, err := os.ReadFile(guidePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(guide)).To(ContainSubstring("@CMDSHAPE.md"))

		res, err := adapter.(interface {
			Uninstall(Context) (InstallResult, error)
		}).Uninstall(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Applied).NotTo(BeZero())
	})

	ginkgo.It("preserves existing guide content when installing", func() {
		Expect(os.MkdirAll(filepath.Join(home, ".claude"), 0o755)).To(Succeed())
		guidePath := filepath.Join(home, ".claude", claudeGuideName)
		original := "# Global Claude Rules\n\nPrefer concise answers.\n"
		Expect(os.WriteFile(guidePath, []byte(original), 0o644)).To(Succeed())

		_, err := adapter.Install(ctx, writeFileWriter)
		Expect(err).NotTo(HaveOccurred())

		guide, err := os.ReadFile(guidePath)
		Expect(err).NotTo(HaveOccurred())

		content := string(guide)
		Expect(content).To(ContainSubstring(original))
		Expect(content).To(ContainSubstring("@CMDSHAPE.md"))
		Expect(strings.Count(content, cmdshapeManagedBlockStart)).To(Equal(1))
	})

	ginkgo.It("reports a second install as noop when artifacts are already current", func() {
		first, err := adapter.Install(ctx, writeFileWriter)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Applied).To(Equal(len(adapter.Plan(ctx))))

		second, err := adapter.Install(ctx, writeFileWriter)
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Applied).To(Equal(0))
		Expect(second.Noop).To(Equal(len(adapter.Plan(ctx))))
	})

	ginkgo.It("returns verification errors for an invalid guide block", func() {
		root := filepath.Join(home, ".claude")
		Expect(os.MkdirAll(filepath.Join(root, "hooks"), 0o755)).To(Succeed())
		hookPath := filepath.Join(root, "hooks", claudeHookScriptName)
		Expect(os.WriteFile(hookPath, []byte("#!/bin/bash\n"), 0o755)).To(Succeed())
		settings := preToolUseCommandSettingsContent(hookPath)
		Expect(os.WriteFile(filepath.Join(root, claudeSettingsName), []byte(settings), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, claudeAwarenessName), []byte("awareness\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, claudeGuideName), []byte("# custom guide\n"), 0o644)).To(Succeed())

		err := adapter.Verify(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing claude managed guide block markers"))
	})

	ginkgo.It("returns verification errors for missing managed artifacts", func() {
		err := adapter.Verify(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing hook script"))
	})

	ginkgo.It("treats uninstall as noop when no managed artifacts exist", func() {
		res, err := adapter.(interface {
			Uninstall(Context) (InstallResult, error)
		}).Uninstall(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Applied).To(Equal(0))
		Expect(res.Noop).To(BeNumerically(">=", 1))
	})
})

var _ = ginkgo.Describe("Claude guide helpers", func() {
	var (
		tmpDir string
		guide  string
	)

	ginkgo.BeforeEach(func() {
		tmpDir = ginkgo.GinkgoT().TempDir()
		guide = filepath.Join(tmpDir, claudeGuideName)
	})

	ginkgo.It("upserts the managed guide block into a missing file", func() {
		updated, err := upsertClaudeGuideBlock(guide)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated).To(ContainSubstring("@CMDSHAPE.md"))
	})

	ginkgo.It("preserves existing guide content when upserting", func() {
		existing := "# Team rules\n\nBe deliberate.\n"
		Expect(os.WriteFile(guide, []byte(existing), 0o644)).To(Succeed())

		updated, err := upsertClaudeGuideBlock(guide)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated).To(ContainSubstring(existing))
		Expect(strings.Count(updated, cmdshapeManagedBlockStart)).To(Equal(1))
	})

	ginkgo.It("removes only the managed guide block when unrelated content exists", func() {
		existing := "# Team rules\n\nBe deliberate.\n"
		Expect(os.WriteFile(guide, []byte(existing+claudeManagedGuideBlock()), 0o644)).To(Succeed())

		out, changed, removeAll, err := removeManagedContextBlock(guide)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())
		Expect(removeAll).To(BeFalse())
		Expect(out).NotTo(ContainSubstring("@CMDSHAPE.md"))
		Expect(out).To(ContainSubstring("Be deliberate."))
	})

	ginkgo.It("can delete a fully managed guide file", func() {
		Expect(os.WriteFile(guide, []byte(claudeManagedGuideBlock()), 0o644)).To(Succeed())

		_, changed, removeAll, err := removeManagedContextBlock(guide)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())
		Expect(removeAll).To(BeTrue())
	})

	ginkgo.It("replaces an existing managed guide block in place", func() {
		initial := "# Team rules\n\n" + claudeManagedGuideBlock() + "\n# Tail\n"
		Expect(os.WriteFile(guide, []byte(initial), 0o644)).To(Succeed())

		updated, err := upsertClaudeGuideBlock(guide)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Count(updated, cmdshapeManagedBlockStart)).To(Equal(1))
		Expect(updated).To(ContainSubstring("# Team rules"))
		Expect(updated).To(ContainSubstring("# Tail"))
		Expect(updated).To(ContainSubstring("@CMDSHAPE.md"))
	})

	ginkgo.It("replaces an empty managed guide block in place", func() {
		initial := "# Team rules\n\n" + cmdshapeManagedBlockStart + "\n" + cmdshapeManagedBlockEnd + "\n# Tail\n"
		Expect(os.WriteFile(guide, []byte(initial), 0o644)).To(Succeed())

		updated, err := upsertClaudeGuideBlock(guide)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Count(updated, cmdshapeManagedBlockStart)).To(Equal(1))
		Expect(updated).To(ContainSubstring("@CMDSHAPE.md"))
		Expect(updated).To(ContainSubstring("# Team rules"))
		Expect(updated).To(ContainSubstring("# Tail"))
	})

	ginkgo.It("returns an error when the guide path cannot be read as a file", func() {
		Expect(os.MkdirAll(guide, 0o755)).To(Succeed())

		_, err := upsertClaudeGuideBlock(guide)
		Expect(err).To(HaveOccurred())
	})

	ginkgo.It("returns an error when the guide file is missing", func() {
		err := verifyClaudeGuideBlock(guide)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing claude guide file"))
	})

	ginkgo.It("returns an error when the managed guide block has no cmdshape reference", func() {
		content := cmdshapeManagedBlockStart + "\n## cmdshape Integration (Managed)\n" + cmdshapeManagedBlockEnd + "\n"
		Expect(os.WriteFile(guide, []byte(content), 0o644)).To(Succeed())

		err := verifyClaudeGuideBlock(guide)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing claude cmdshape guide reference"))
	})
})

var _ = ginkgo.Describe("Claude hook removal helpers", func() {
	var (
		tmpDir string
	)

	ginkgo.BeforeEach(func() {
		tmpDir = ginkgo.GinkgoT().TempDir()
	})

	ginkgo.It("removes the managed pre-tool-use hook and deletes empty settings files", func() {
		settings := filepath.Join(tmpDir, "settings.json")
		hook := filepath.Join(tmpDir, "cmdshape-rewrite.sh")
		content := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"` + strings.ReplaceAll(hook, "\\", "\\\\") + `"}]}]}}`
		Expect(os.WriteFile(settings, []byte(content), 0o644)).To(Succeed())

		changed, err := removePreToolUseCommandHook(settings, hook)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())

		_, err = os.Stat(settings)
		Expect(err).To(MatchError(os.ErrNotExist))
	})

	ginkgo.It("does nothing when the settings file is missing", func() {
		settings := filepath.Join(tmpDir, "settings.json")
		hook := filepath.Join(tmpDir, "cmdshape-rewrite.sh")

		changed, err := removePreToolUseCommandHook(settings, hook)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())
	})

	ginkgo.It("preserves invalid user settings files and returns an actionable error", func() {
		settings := filepath.Join(tmpDir, "settings.json")
		hook := filepath.Join(tmpDir, "cmdshape-rewrite.sh")
		original := []byte("{invalid")
		Expect(os.WriteFile(settings, original, 0o644)).To(Succeed())

		changed, err := removePreToolUseCommandHook(settings, hook)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid"))
		Expect(changed).To(BeFalse())

		raw, readErr := os.ReadFile(settings)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(raw).To(Equal(original))
	})

	ginkgo.It("tolerates removing a missing file", func() {
		_, err := removeFileIfExists(filepath.Join(tmpDir, "missing"))
		Expect(err).NotTo(HaveOccurred())
	})

	ginkgo.It("returns an error when removing a directory as a file", func() {
		dir := filepath.Join(tmpDir, "dir")
		Expect(os.MkdirAll(dir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dir, "nested"), []byte("x"), 0o644)).To(Succeed())

		_, err := removeFileIfExists(dir)
		Expect(err).To(HaveOccurred())
	})

	ginkgo.It("returns an error when removing managed artifacts fails", func() {
		dir := filepath.Join(tmpDir, "dir")
		Expect(os.MkdirAll(dir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dir, "nested"), []byte("x"), 0o644)).To(Succeed())

		err := removeClaudeArtifacts(&InstallResult{}, dir)
		Expect(err).To(HaveOccurred())
	})

	ginkgo.It("returns an error when uninstalling guide changes cannot be applied", func() {
		guideDir := filepath.Join(tmpDir, "guide")
		Expect(os.MkdirAll(guideDir, 0o755)).To(Succeed())

		err := uninstallClaudeGuide(&InstallResult{}, guideDir)
		Expect(err).To(HaveOccurred())
	})

	ginkgo.It("returns an error when uninstalling Claude settings cannot read the settings file", func() {
		settingsDir := filepath.Join(tmpDir, "settings")
		Expect(os.MkdirAll(settingsDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(settingsDir, "nested"), []byte("x"), 0o644)).To(Succeed())

		res := &InstallResult{}
		err := uninstallClaudeSettings(res, settingsDir, filepath.Join(tmpDir, claudeHookScriptName))
		Expect(err).To(HaveOccurred())
		Expect(res.Applied).To(BeZero())
		Expect(res.Noop).To(BeZero())
	})

	ginkgo.It("returns an error when uninstalling guide changes through a symlinked file", func() {
		outside := filepath.Join(tmpDir, "outside-guide.md")
		guideLink := filepath.Join(tmpDir, claudeGuideName)
		Expect(os.WriteFile(outside, []byte("# Team rules\n\n"+claudeManagedGuideBlock()+"\n# Tail\n"), 0o644)).To(Succeed())
		if err := os.Symlink(outside, guideLink); err != nil {
			ginkgo.Skip("symlink creation unavailable: " + err.Error())
		}

		res := &InstallResult{}
		err := uninstallClaudeGuide(res, guideLink)
		Expect(err).To(HaveOccurred())
		Expect(res.Applied).To(BeZero())
		Expect(res.Noop).To(BeZero())
	})

	ginkgo.DescribeTable("propagates Claude uninstall helper failures",
		func(setup func(root string), expected string) {
			root := filepath.Join(tmpDir, ".claude")
			Expect(os.MkdirAll(filepath.Join(root, "hooks"), 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(root, "hooks", claudeHookScriptName), []byte("#!/bin/sh\n"), 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(root, claudeAwarenessName), []byte("awareness\n"), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(root, claudeSettingsName), []byte("{}\n"), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(root, claudeGuideName), []byte("# Team rules\n\n"+claudeManagedGuideBlock()), 0o644)).To(Succeed())
			setup(root)

			_, err := (ClaudeAdapter{}).Uninstall(Context{HomeDir: tmpDir, ScopeRoot: filepath.Join(tmpDir, "repo")})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(expected))
		},
		ginkgo.Entry("from settings cleanup", func(root string) {
			settingsPath := filepath.Join(root, claudeSettingsName)
			Expect(os.Remove(settingsPath)).To(Succeed())
			Expect(os.MkdirAll(settingsPath, 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(settingsPath, "nested"), []byte("x"), 0o644)).To(Succeed())
		}, "read"),
		ginkgo.Entry("from guide cleanup", func(root string) {
			guidePath := filepath.Join(root, claudeGuideName)
			outside := filepath.Join(tmpDir, "outside-guide.md")
			Expect(os.WriteFile(outside, []byte("# Team rules\n\n"+claudeManagedGuideBlock()+"\n# Tail\n"), 0o644)).To(Succeed())
			Expect(os.Remove(guidePath)).To(Succeed())
			if err := os.Symlink(outside, guidePath); err != nil {
				ginkgo.Skip("symlink creation unavailable: " + err.Error())
			}
		}, "symlink"),
	)
})

var _ = ginkgo.Describe("Claude hook script content", func() {
	ginkgo.It("contains the expected runtime guards and payload flow", func() {
		script := bashRewriteHookScriptContent("claude", "cmdshape-claude-hook.log")
		Expect(script).NotTo(ContainSubstring("command -v jq"))
		Expect(script).To(ContainSubstring(`command -v cmdshape`))
		Expect(script).To(ContainSubstring(`LOG_FILE="${TMPDIR:-/tmp}/cmdshape-claude-hook.log"`))
		Expect(script).NotTo(ContainSubstring(`skip-complex-shape`))

		for _, reason := range []string{
			"skip-no-cmdshape",
			"skip-empty-input",
			"skip-no-command",
			"skip-empty-rewrite",
			"skip-no-change",
			"skip-invalid-shell",
		} {
			Expect(script).To(ContainSubstring(reason))
		}
		Expect(script).To(ContainSubstring(`REWRITTEN_CMD="$(rewrite_command "$CMD")"`))
		Expect(script).To(ContainSubstring(`ESCAPED_CMD="$(json_escape "$REWRITTEN_CMD")"`))
		Expect(script).To(ContainSubstring(`updatedInput\":{\"command\":\"$ESCAPED_CMD\"}`))
	})
})

var _ = ginkgo.Describe("Claude hook script execution", func() {
	type hookCase struct {
		name         string
		input        string
		withCmdshape bool
		wantLog      string
		wantCommand  string
		wantNoOutput bool
	}

	ginkgo.DescribeTable("executing the rewrite hook",
		func(tc hookCase) {
			stdout, logOutput := runClaudeHookScriptSpec(tc.input, tc.withCmdshape)

			if tc.wantLog != "" {
				Expect(logOutput).To(ContainSubstring(tc.wantLog))
			}
			if tc.wantNoOutput {
				Expect(strings.TrimSpace(stdout)).To(BeEmpty())
				return
			}

			Expect(strings.TrimSpace(logOutput)).To(BeEmpty())
			got := decodeClaudeHookOutput(ginkgo.GinkgoT(), stdout)
			Expect(got).To(Equal(tc.wantCommand))
		},
		ginkgo.Entry("missing cmdshape dependency", hookCase{name: "missing cmdshape dependency", input: `{"tool_input":{"command":"pwd && ls"}}`, wantLog: "skip-no-cmdshape", wantNoOutput: true}),
		ginkgo.Entry("empty input", hookCase{name: "empty input", input: "", withCmdshape: true, wantLog: "skip-empty-input", wantNoOutput: true}),
		ginkgo.Entry("missing command field", hookCase{name: "missing command field", input: `{"tool_input":{}}`, withCmdshape: true, wantLog: "skip-no-command", wantNoOutput: true}),
		ginkgo.Entry("whitespace command remains untouched", hookCase{name: "whitespace command remains untouched", input: `{"tool_input":{"command":"   "}}`, withCmdshape: true, wantLog: "skip-no-change", wantNoOutput: true}),
		ginkgo.Entry("already prefixed command", hookCase{name: "already prefixed command", input: `{"tool_input":{"command":"cmdshape pwd"}}`, withCmdshape: true, wantLog: "skip-no-change", wantNoOutput: true}),
		ginkgo.Entry("malformed chain remains untouched", hookCase{name: "malformed chain remains untouched", input: `{"tool_input":{"command":"git status &&"}}`, withCmdshape: true, wantLog: "skip-no-change", wantNoOutput: true}),
		ginkgo.Entry("relative executable rewrite", hookCase{name: "relative executable rewrite", input: `{"tool_input":{"command":"./gradlew --version"}}`, withCmdshape: true, wantCommand: "cmdshape ./gradlew --version"}),
		ginkgo.Entry("simple chained rewrite", hookCase{name: "simple chained rewrite", input: `{"tool_input":{"command":"git status && ls"}}`, withCmdshape: true, wantCommand: "cmdshape git status && cmdshape ls"}),
		ginkgo.Entry("double quoted rewrite", hookCase{name: "double quoted rewrite", input: `{"tool_input":{"command":"git commit -m \"test\""}}`, withCmdshape: true, wantCommand: `cmdshape git commit -m "test"`}),
		ginkgo.Entry("single quoted rewrite", hookCase{name: "single quoted rewrite", input: `{"tool_input":{"command":"git commit -m 'test'"}}`, withCmdshape: true, wantCommand: `cmdshape git commit -m 'test'`}),
		ginkgo.Entry("backslash command rewrite", hookCase{name: "backslash command rewrite", input: `{"tool_input":{"command":"ls foo\\ bar"}}`, withCmdshape: true, wantCommand: `cmdshape ls foo\ bar`}),
		ginkgo.Entry("command substitution remains untouched", hookCase{name: "command substitution remains untouched", input: `{"tool_input":{"command":"git add $(pwd)"}}`, withCmdshape: true, wantLog: "skip-no-change", wantNoOutput: true}),
		ginkgo.Entry("parameter expansion remains untouched", hookCase{name: "parameter expansion remains untouched", input: `{"tool_input":{"command":"git add ${HOME}"}}`, withCmdshape: true, wantLog: "skip-no-change", wantNoOutput: true}),
		ginkgo.Entry("heredoc remains untouched", hookCase{name: "heredoc remains untouched", input: `{"tool_input":{"command":"cat <<EOF"}}`, withCmdshape: true, wantLog: "skip-no-change", wantNoOutput: true}),
		ginkgo.Entry("quoted chained rewrite", hookCase{name: "quoted chained rewrite", input: `{"tool_input":{"command":"git commit -m \"msg\" && git status"}}`, withCmdshape: true, wantCommand: `cmdshape git commit -m "msg" && cmdshape git status`}),
		ginkgo.Entry("pipeline remains entirely untouched", hookCase{name: "pipeline remains entirely untouched", input: `{"tool_input":{"command":"cmdshape git status && ls | head"}}`, withCmdshape: true, wantLog: "skip-no-change", wantNoOutput: true}),
	)
})

var _ = ginkgo.Describe("Claude artifact installation helpers", func() {
	var (
		tmpDir string
	)

	ginkgo.BeforeEach(func() {
		tmpDir = ginkgo.GinkgoT().TempDir()
	})

	ginkgo.It("returns writer errors when installing artifacts", func() {
		item := PlannedArtifact{
			Kind:    ArtifactHook,
			Path:    filepath.Join(tmpDir, "hooks", claudeHookScriptName),
			Content: "#!/bin/bash\n",
			Perm:    0o755,
		}

		_, err := installClaudeArtifact(item, func(path string, data []byte, perm os.FileMode) (bool, error) {
			return false, os.ErrPermission
		})
		Expect(err).To(HaveOccurred())
	})

	ginkgo.It("returns chmod errors for hook artifacts when the path becomes a directory", func() {
		hookPath := filepath.Join(tmpDir, "hooks", claudeHookScriptName)
		item := PlannedArtifact{
			Kind:    ArtifactHook,
			Path:    hookPath,
			Content: "#!/bin/bash\n",
			Perm:    0o755,
		}

		_, err := installClaudeArtifact(item, func(path string, data []byte, perm os.FileMode) (bool, error) {
			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			return true, nil
		})
		Expect(err).To(HaveOccurred())
	})

	ginkgo.It("treats awareness artifacts as plain content writes", func() {
		item := PlannedArtifact{
			Kind:    ArtifactAwareness,
			Path:    filepath.Join(tmpDir, claudeAwarenessName),
			Content: "awareness\n",
			Perm:    0o644,
		}

		changed, err := installClaudeArtifact(item, writeFileWriter)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())
	})
})

func runClaudeHookScriptSpec(input string, withCmdshape bool) (string, string) {
	result := runHookScript(ginkgo.GinkgoT(), "cmdshape-rewrite.sh", "cmdshape-claude-hook.log", bashRewriteHookScriptContent("claude", "cmdshape-claude-hook.log"), input, withCmdshape)
	Expect(result.exitCode).To(Equal(0), result.stderr)
	return result.stdout, result.log
}
