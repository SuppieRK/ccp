package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("WindsurfAdapter", func() {
	var (
		tmpDir    string
		scopeRoot string
		home      string
		ctx       Context
		adapter   WindsurfAdapter
	)

	ginkgo.BeforeEach(func() {
		tmpDir = ginkgo.GinkgoT().TempDir()
		scopeRoot = filepath.Join(tmpDir, "repo")
		home = filepath.Join(tmpDir, "home")
		ctx = Context{ScopeRoot: scopeRoot, HomeDir: home}
		adapter = WindsurfAdapter{}

		Expect(os.MkdirAll(filepath.Join(scopeRoot, ".windsurf"), 0o755)).To(Succeed())
		Expect(os.MkdirAll(home, 0o755)).To(Succeed())
	})

	ginkgo.It("installs, verifies, and uninstalls the managed hook integration", func() {
		Expect(adapter.ID()).To(Equal("windsurf"))
		Expect(adapter.DetectRoot(ctx.ScopeRoot)).To(ContainSubstring(".windsurf"))

		plan := adapter.Plan(ctx)
		Expect(plan).To(HaveLen(2))
		Expect(plan[0].Path).To(HaveSuffix(filepath.Join(".codeium", "windsurf", "hooks", "ccp-block.sh")))
		Expect(plan[1].Path).To(HaveSuffix(filepath.Join(".codeium", "windsurf", "hooks.json")))

		_, err := adapter.Install(ctx, writeFileWriter)
		Expect(err).NotTo(HaveOccurred())
		Expect(adapter.Verify(ctx)).To(Succeed())

		res, err := adapter.Uninstall(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Applied).To(Equal(2))
	})
})

var _ = ginkgo.Describe("windsurf hooks config helpers", func() {
	var (
		tmpDir      string
		hooksPath   string
		managedHook string
	)

	ginkgo.BeforeEach(func() {
		tmpDir = ginkgo.GinkgoT().TempDir()
		hooksPath = filepath.Join(tmpDir, "hooks.json")
		managedHook = filepath.Join(tmpDir, "ccp-block.sh")
	})

	ginkgo.It("upserts and verifies the managed hook entry", func() {
		updated, err := upsertWindsurfHooksConfig(hooksPath, managedHook)
		Expect(err).NotTo(HaveOccurred())

		var root map[string]any
		Expect(json.Unmarshal([]byte(updated), &root)).To(Succeed())
		Expect(windsurfHookEntriesContain(normalizeWindsurfHookEntries(root["pre_run_command"]), managedHook)).To(BeTrue())

		Expect(os.WriteFile(hooksPath, []byte(updated), 0o644)).To(Succeed())

		ok, err := windsurfHooksConfigHasEntry(hooksPath, managedHook)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
	})

	ginkgo.It("removes only the managed hook when other hooks remain", func() {
		otherHook := filepath.Join(tmpDir, "other.sh")
		content := "{\n  \"pre_run_command\": [\n    {\n      \"name\": \"ccp-pre-run-command\",\n      \"command\": \"" + strings.ReplaceAll(managedHook, "\\", "\\\\") + "\",\n      \"enabled\": true\n    },\n    {\n      \"name\": \"other\",\n      \"command\": \"" + strings.ReplaceAll(otherHook, "\\", "\\\\") + "\",\n      \"enabled\": true\n    }\n  ]\n}\n"
		Expect(os.WriteFile(hooksPath, []byte(content), 0o644)).To(Succeed())

		removed, changed, removeAll, err := removeWindsurfHooksConfig(hooksPath, managedHook)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())
		Expect(removeAll).To(BeFalse())

		var root map[string]any
		Expect(json.Unmarshal([]byte(removed), &root)).To(Succeed())
		entries := normalizeWindsurfHookEntries(root["pre_run_command"])
		Expect(windsurfHookEntriesContain(entries, managedHook)).To(BeFalse())
		Expect(windsurfHookEntriesContain(entries, otherHook)).To(BeTrue())
	})

	ginkgo.It("signals remove-all when the managed hook is the only remaining entry", func() {
		content := "{\n  \"pre_run_command\": [\n    {\n      \"name\": \"ccp-pre-run-command\",\n      \"command\": \"" + strings.ReplaceAll(managedHook, "\\", "\\\\") + "\",\n      \"enabled\": true\n    }\n  ]\n}\n"
		Expect(os.WriteFile(hooksPath, []byte(content), 0o644)).To(Succeed())

		_, changed, removeAll, err := removeWindsurfHooksConfig(hooksPath, managedHook)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())
		Expect(removeAll).To(BeTrue())
	})

	ginkgo.It("returns no hook entries for unexpected input types", func() {
		Expect(normalizeWindsurfHookEntries("unexpected")).To(BeNil())
	})
})

var _ = ginkgo.Describe("Windsurf hook script", func() {
	ginkgo.DescribeTable("handles expected runtime branches",
		func(input string, withCCP bool, wantLog string, wantExitCode int, wantStderr string) {
			result := runHookScript(ginkgo.GinkgoT(), windsurfHookScriptName, "ccp-windsurf-hook.log", windsurfHookScriptContent(), input, withCCP)
			Expect(result.exitCode).To(Equal(wantExitCode))
			if wantLog != "" {
				Expect(result.log).To(ContainSubstring(wantLog))
			}
			if wantStderr != "" {
				Expect(result.stderr).To(ContainSubstring(wantStderr))
			}
		},
		ginkgo.Entry("missing ccp", `{"command":"pwd"}`, false, "skip-no-ccp", 0, ""),
		ginkgo.Entry("empty input", "", true, "skip-empty-input", 0, ""),
		ginkgo.Entry("missing command", `{"tool_input":{}}`, true, "skip-no-command", 0, ""),
		ginkgo.Entry("already prefixed", `{"command":"ccp pwd"}`, true, "skip-already-prefixed", 0, ""),
		ginkgo.Entry("block", `{"command":"pwd"}`, true, "", 2, "Retry as: ccp pwd"),
	)

	ginkgo.It("contains the canonical script content markers", func() {
		content := windsurfHookScriptContent()
		for _, needle := range []string{
			"generated by ccp init for windsurf",
			"pre_run_command hook",
			"Use ccp as the command prefix for shell commands",
			"exit 2",
			`LOG_FILE="${TMPDIR:-/tmp}/ccp-windsurf-hook.log"`,
		} {
			Expect(content).To(ContainSubstring(needle))
		}
	})
})
