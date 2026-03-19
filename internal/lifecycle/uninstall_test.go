package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go-command-compression-proxy/internal/lifecycle/agents"
)

const (
	toolsFlag      = "--tools"
	claudeDirName  = ".claude"
	claudeHookName = "ccp-rewrite.sh"
)

type uninstallRemovalCase struct {
	name      string
	tool      string
	scope     string
	setupDirs []string
	removed   []string
}

type uninstallPreserveCase struct {
	name   string
	tool   string
	scope  string
	setup  func(lifecycleWorkspace)
	assert func(lifecycleWorkspace)
}

type uninstallAutodetectCase struct {
	name      string
	tool      string
	scope     string
	setupDirs []string
	removed   string
}

var _ = Describe("uninstall", func() {
	DescribeTable("removing managed artifacts",
		func(tc uninstallRemovalCase) {
			ws := newUninstallWorkspace(tc.scope)
			for _, dir := range tc.setupDirs {
				Expect(os.MkdirAll(expandUninstallPath(ws, dir), 0o755)).To(Succeed())
			}

			Expect(RunInit(toolsArgs(tc.tool))).To(Succeed())
			Expect(RunUninstall(toolsArgs(tc.tool))).To(Succeed())

			for _, path := range tc.removed {
				expectMissingPath(expandUninstallPath(ws, path))
			}
		},
		Entry("codex", uninstallRemovalCase{
			name:    "codex",
			tool:    "codex",
			scope:   "work",
			removed: []string{"{home}/.codex/AGENTS.md"},
		}),
		Entry("github-copilot", uninstallRemovalCase{
			name:    "github-copilot",
			tool:    "github-copilot",
			scope:   "work",
			removed: []string{"{home}/.copilot/copilot-instructions.md"},
		}),
		Entry("aider", uninstallRemovalCase{
			name:  "aider",
			tool:  "aider",
			scope: "root",
			removed: []string{
				"{home}/.aider.conf.yml",
				"{home}/.aider.rules.md",
			},
		}),
		Entry("gemini", uninstallRemovalCase{
			name:    "gemini",
			tool:    "gemini",
			scope:   "work",
			removed: []string{"{home}/.gemini/GEMINI.md"},
		}),
		Entry("qwen", uninstallRemovalCase{
			name:      "qwen",
			tool:      "qwen",
			scope:     "root",
			setupDirs: []string{"{root}/.qwen"},
			removed: []string{
				"{home}/.qwen/AGENTS.md",
				"{home}/.qwen/settings.json",
			},
		}),
		Entry("qoder", uninstallRemovalCase{
			name:      "qoder",
			tool:      "qoder",
			scope:     "root",
			setupDirs: []string{"{root}/.qoder"},
			removed:   []string{"{home}/.qoder/AGENTS.md"},
		}),
		Entry("factory", uninstallRemovalCase{
			name:      "factory",
			tool:      "factory",
			scope:     "root",
			setupDirs: []string{"{root}/.factory"},
			removed:   []string{"{home}/.factory/AGENTS.md"},
		}),
		Entry("auggie", uninstallRemovalCase{
			name:      "auggie",
			tool:      "auggie",
			scope:     "root",
			setupDirs: []string{"{root}/.augment"},
			removed:   []string{"{root}/AGENTS.md"},
		}),
		Entry("codebuddy", uninstallRemovalCase{
			name:      "codebuddy",
			tool:      "codebuddy",
			scope:     "root",
			setupDirs: []string{"{root}/.codebuddy"},
			removed: []string{
				"{home}/.codebuddy/hooks/ccp-rewrite.sh",
				"{home}/.codebuddy/settings.json",
			},
		}),
		Entry("crush", uninstallRemovalCase{
			name:      "crush",
			tool:      "crush",
			scope:     "root",
			setupDirs: []string{"{root}/.crush"},
			removed: []string{
				"{home}/.config/crush/CRUSH.md",
				"{home}/.config/crush/crush.json",
			},
		}),
		Entry("iflow", uninstallRemovalCase{
			name:      "iflow",
			tool:      "iflow",
			scope:     "root",
			setupDirs: []string{"{root}/.iflow"},
			removed:   []string{"{home}/.iflow/IFLOW.md"},
		}),
		Entry("pi", uninstallRemovalCase{
			name:      "pi",
			tool:      "pi",
			scope:     "root",
			setupDirs: []string{"{root}/.pi"},
			removed:   []string{"{root}/AGENTS.md"},
		}),
		Entry("cursor", uninstallRemovalCase{
			name:      "cursor",
			tool:      "cursor",
			scope:     "root",
			setupDirs: []string{"{root}/.cursor"},
			removed:   []string{"{root}/.cursor/rules/ccp.mdc"},
		}),
		Entry("amazon-q", uninstallRemovalCase{
			name:      "amazon-q",
			tool:      "amazon-q",
			scope:     "root",
			setupDirs: []string{"{root}/.amazonq"},
			removed:   []string{"{root}/.amazonq/rules/ccp.md"},
		}),
		Entry("antigravity", uninstallRemovalCase{
			name:      "antigravity",
			tool:      "antigravity",
			scope:     "root",
			setupDirs: []string{"{root}/.agent"},
			removed:   []string{"{home}/.gemini/GEMINI.md"},
		}),
		Entry("kiro", uninstallRemovalCase{
			name:      "kiro",
			tool:      "kiro",
			scope:     "root",
			setupDirs: []string{"{root}/.kiro"},
			removed:   []string{"{home}/.kiro/steering/AGENTS.md"},
		}),
		Entry("kilocode", uninstallRemovalCase{
			name:      "kilocode",
			tool:      "kilocode",
			scope:     "root",
			setupDirs: []string{"{root}/.kilocode"},
			removed:   []string{"{home}/.config/kilocode/plugins/ccp-rewrite.js"},
		}),
		Entry("roocode", uninstallRemovalCase{
			name:      "roocode",
			tool:      "roocode",
			scope:     "root",
			setupDirs: []string{"{root}/.roo"},
			removed:   []string{"{home}/.roo/rules/ccp.md"},
		}),
		Entry("costrict alias", uninstallRemovalCase{
			name:      "costrict",
			tool:      "costrict",
			scope:     "root",
			setupDirs: []string{"{root}/.roo"},
			removed:   []string{"{home}/.roo/rules/ccp.md"},
		}),
		Entry("trae", uninstallRemovalCase{
			name:      "trae",
			tool:      "trae",
			scope:     "root",
			setupDirs: []string{"{root}/.trae"},
			removed:   []string{"{root}/.trae/rules/ccp.md"},
		}),
		Entry("opencode", uninstallRemovalCase{
			name:    "opencode",
			tool:    "opencode",
			scope:   "root",
			removed: []string{"{home}/.config/opencode/plugins/ccp-rewrite.js"},
		}),
		Entry("claude", uninstallRemovalCase{
			name:  "claude",
			tool:  "claude",
			scope: "work",
			removed: []string{
				"{home}/" + claudeDirName + "/hooks/" + claudeHookName,
				"{home}/" + claudeDirName + "/settings.json",
				"{home}/" + claudeDirName + "/CCP.md",
			},
		}),
	)

	DescribeTable("preserving non-CCP content",
		func(tc uninstallPreserveCase) {
			ws := newUninstallWorkspace(tc.scope)
			tc.setup(ws)
			Expect(RunUninstall(toolsArgs(tc.tool))).To(Succeed())
			tc.assert(ws)
		},
		Entry("claude preserves non-CCP hooks", uninstallPreserveCase{
			name:  "claude preserves non-CCP hooks",
			tool:  "claude",
			scope: "work",
			setup: func(ws lifecycleWorkspace) {
				Expect(os.MkdirAll(filepath.Join(ws.home, claudeDirName, "hooks"), 0o755)).To(Succeed())

				settingsPath := filepath.Join(ws.home, claudeDirName, "settings.json")
				hookPath := filepath.Join(ws.home, claudeDirName, "hooks", claudeHookName)
				otherPath := filepath.Join(ws.home, claudeDirName, "hooks", "other.sh")
				Expect(os.WriteFile(hookPath, []byte("#!/bin/sh\n"), 0o755)).To(Succeed())
				Expect(os.WriteFile(otherPath, []byte("#!/bin/sh\n"), 0o755)).To(Succeed())

				settings := "{\n  \"hooks\": {\n    \"PreToolUse\": [\n      {\n        \"matcher\": \"Bash\",\n        \"hooks\": [\n          {\"type\": \"command\", \"command\": \"" + strings.ReplaceAll(hookPath, "\\", "\\\\") + "\"}\n        ]\n      },\n      {\n        \"matcher\": \"Bash\",\n        \"hooks\": [\n          {\"type\": \"command\", \"command\": \"" + strings.ReplaceAll(otherPath, "\\", "\\\\") + "\"}\n        ]\n      }\n    ]\n  }\n}\n"
				Expect(os.WriteFile(settingsPath, []byte(settings), 0o644)).To(Succeed())
			},
			assert: func(ws lifecycleWorkspace) {
				hookPath := filepath.Join(ws.home, claudeDirName, "hooks", claudeHookName)
				settingsPath := filepath.Join(ws.home, claudeDirName, "settings.json")
				expectMissingPath(hookPath)

				b, err := os.ReadFile(settingsPath)
				Expect(err).NotTo(HaveOccurred())

				got := string(b)
				Expect(got).NotTo(ContainSubstring(claudeHookName))
				Expect(got).To(ContainSubstring("other.sh"))
			},
		}),
		Entry("github-copilot preserves user content", blockPreserveCase("github-copilot", "work", func(ws lifecycleWorkspace) string {
			return filepath.Join(ws.home, ".copilot", "copilot-instructions.md")
		})),
		Entry("gemini preserves user content", blockPreserveCase("gemini", "work", func(ws lifecycleWorkspace) string {
			return filepath.Join(ws.home, ".gemini", "GEMINI.md")
		})),
		Entry("qoder preserves user content", blockPreserveCase("qoder", "root", func(ws lifecycleWorkspace) string {
			Expect(os.MkdirAll(filepath.Join(ws.root, ".qoder"), 0o755)).To(Succeed())
			return filepath.Join(ws.home, ".qoder", "AGENTS.md")
		})),
		Entry("factory preserves user content", blockPreserveCase("factory", "root", func(ws lifecycleWorkspace) string {
			Expect(os.MkdirAll(filepath.Join(ws.root, ".factory"), 0o755)).To(Succeed())
			Expect(os.MkdirAll(filepath.Join(ws.home, ".factory"), 0o755)).To(Succeed())
			return filepath.Join(ws.home, ".factory", "AGENTS.md")
		})),
		Entry("auggie preserves user content", blockPreserveCase("auggie", "root", func(ws lifecycleWorkspace) string {
			Expect(os.MkdirAll(filepath.Join(ws.root, ".augment"), 0o755)).To(Succeed())
			return filepath.Join(ws.root, "AGENTS.md")
		})),
		Entry("iflow preserves user content", blockPreserveCase("iflow", "root", func(ws lifecycleWorkspace) string {
			Expect(os.MkdirAll(filepath.Join(ws.root, ".iflow"), 0o755)).To(Succeed())
			Expect(os.MkdirAll(filepath.Join(ws.home, ".iflow"), 0o755)).To(Succeed())
			return filepath.Join(ws.home, ".iflow", "IFLOW.md")
		})),
		Entry("antigravity preserves gemini-family content", blockPreserveCase("antigravity", "root", func(ws lifecycleWorkspace) string {
			Expect(os.MkdirAll(filepath.Join(ws.root, ".agent"), 0o755)).To(Succeed())
			Expect(os.MkdirAll(filepath.Join(ws.home, ".gemini"), 0o755)).To(Succeed())
			return filepath.Join(ws.home, ".gemini", "GEMINI.md")
		})),
		Entry("pi preserves non-CCP content", uninstallPreserveCase{
			name:  "pi preserves non-CCP content",
			tool:  "pi",
			scope: "root",
			setup: func(ws lifecycleWorkspace) {
				Expect(os.MkdirAll(filepath.Join(ws.root, ".pi"), 0o755)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(ws.root, "AGENTS.md"), []byte("team notes\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nmanaged content\n<!-- END: CCP MANAGED BLOCK -->\n"), 0o644)).To(Succeed())
			},
			assert: func(ws lifecycleWorkspace) {
				got, err := os.ReadFile(filepath.Join(ws.root, "AGENTS.md"))
				Expect(err).NotTo(HaveOccurred())
				Expect(strings.TrimSpace(string(got))).To(Equal("team notes"))
			},
		}),
		Entry("aider preserves other config entries", uninstallPreserveCase{
			name:  "aider preserves other config entries",
			tool:  "aider",
			scope: "root",
			setup: func(ws lifecycleWorkspace) {
				configPath := filepath.Join(ws.home, ".aider.conf.yml")
				rulesPath := filepath.Join(ws.home, ".aider.rules.md")
				Expect(os.WriteFile(configPath, []byte("read:\n  - CONVENTIONS.md\nmodel: sonnet\n"), 0o644)).To(Succeed())
				Expect(os.WriteFile(rulesPath, []byte("# User Notes\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nmanaged content\n<!-- END: CCP MANAGED BLOCK -->\n"), 0o644)).To(Succeed())
				Expect(RunInit(toolsArgs("aider"))).To(Succeed())
			},
			assert: func(ws lifecycleWorkspace) {
				configPath := filepath.Join(ws.home, ".aider.conf.yml")
				rulesPath := filepath.Join(ws.home, ".aider.rules.md")
				b, err := os.ReadFile(configPath)
				Expect(err).NotTo(HaveOccurred())

				got := string(b)
				Expect(got).NotTo(ContainSubstring(".aider.rules.md"))
				Expect(got).To(ContainSubstring("CONVENTIONS.md"))
				Expect(got).To(ContainSubstring("model: sonnet"))

				rulesBytes, err := os.ReadFile(rulesPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(rulesBytes)).NotTo(ContainSubstring("managed content"))
				Expect(string(rulesBytes)).To(ContainSubstring("# User Notes"))
			},
		}),
		Entry("qwen preserves non-CCP content", uninstallPreserveCase{
			name:  "qwen preserves non-CCP content",
			tool:  "qwen",
			scope: "root",
			setup: func(ws lifecycleWorkspace) {
				Expect(os.MkdirAll(filepath.Join(ws.root, ".qwen"), 0o755)).To(Succeed())

				agentsPath := filepath.Join(ws.home, ".qwen", "AGENTS.md")
				Expect(os.MkdirAll(filepath.Dir(agentsPath), 0o755)).To(Succeed())

				content := "# User Content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nmanaged content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"
				Expect(os.WriteFile(agentsPath, []byte(content), 0o644)).To(Succeed())

				settingsPath := filepath.Join(ws.home, ".qwen", "settings.json")
				Expect(os.WriteFile(settingsPath, []byte("{\n  \"theme\": \"light\",\n  \"context\": {\n    \"fileName\": \"AGENTS.md\"\n  }\n}\n"), 0o644)).To(Succeed())
			},
			assert: func(ws lifecycleWorkspace) {
				agentsPath := filepath.Join(ws.home, ".qwen", "AGENTS.md")
				settingsPath := filepath.Join(ws.home, ".qwen", "settings.json")
				b, err := os.ReadFile(agentsPath)
				Expect(err).NotTo(HaveOccurred())

				got := string(b)
				Expect(got).NotTo(ContainSubstring("managed content"))
				Expect(got).To(ContainSubstring("# User Content"))
				Expect(got).To(ContainSubstring("# Tail"))

				settingsBytes, err := os.ReadFile(settingsPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(settingsBytes)).To(ContainSubstring(`"theme": "light"`))
				Expect(string(settingsBytes)).NotTo(ContainSubstring(`"fileName": "AGENTS.md"`))
			},
		}),
		Entry("crush preserves non-CCP content", uninstallPreserveCase{
			name:  "crush preserves non-CCP content",
			tool:  "crush",
			scope: "root",
			setup: func(ws lifecycleWorkspace) {
				Expect(os.MkdirAll(filepath.Join(ws.root, ".crush"), 0o755)).To(Succeed())
				Expect(os.MkdirAll(filepath.Join(ws.home, ".config", "crush"), 0o755)).To(Succeed())

				agentsPath := filepath.Join(ws.home, ".config", "crush", "CRUSH.md")
				Expect(os.WriteFile(agentsPath, []byte("# User Content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nmanaged content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"), 0o644)).To(Succeed())

				configPath := filepath.Join(ws.home, ".config", "crush", "crush.json")
				contextRef := strings.ReplaceAll(agentsPath, "\\", "\\\\")
				config := "{\n  \"theme\": \"dark\",\n  \"options\": {\n    \"context_paths\": [\n      \"" + contextRef + "\"\n    ]\n  }\n}\n"
				Expect(os.WriteFile(configPath, []byte(config), 0o644)).To(Succeed())
			},
			assert: func(ws lifecycleWorkspace) {
				agentsPath := filepath.Join(ws.home, ".config", "crush", "CRUSH.md")
				configPath := filepath.Join(ws.home, ".config", "crush", "crush.json")
				b, err := os.ReadFile(agentsPath)
				Expect(err).NotTo(HaveOccurred())

				got := string(b)
				Expect(got).NotTo(ContainSubstring("managed content"))
				Expect(got).To(ContainSubstring("# User Content"))
				Expect(got).To(ContainSubstring("# Tail"))

				cfg, err := os.ReadFile(configPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(cfg)).To(ContainSubstring(`"theme": "dark"`))
				Expect(string(cfg)).NotTo(ContainSubstring(strings.ReplaceAll(agentsPath, "\\", "\\\\")))
			},
		}),
		Entry("codebuddy preserves unrelated settings", uninstallPreserveCase{
			name:  "codebuddy preserves unrelated settings",
			tool:  "codebuddy",
			scope: "root",
			setup: func(ws lifecycleWorkspace) {
				Expect(os.MkdirAll(filepath.Join(ws.root, ".codebuddy"), 0o755)).To(Succeed())
				Expect(os.MkdirAll(filepath.Join(ws.home, ".codebuddy"), 0o755)).To(Succeed())

				settingsPath := filepath.Join(ws.home, ".codebuddy", "settings.json")
				hookPath := filepath.Join(ws.home, ".codebuddy", "hooks", "ccp-rewrite.sh")
				escapedHook := strings.ReplaceAll(hookPath, "\\", "\\\\")
				settings := "{\n  \"theme\": \"light\",\n  \"hooks\": {\n    \"PreToolUse\": [\n      {\n        \"matcher\": \"Bash\",\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"" + escapedHook + "\"\n          }\n        ]\n      }\n    ]\n  }\n}\n"
				Expect(os.MkdirAll(filepath.Dir(hookPath), 0o755)).To(Succeed())
				Expect(os.WriteFile(settingsPath, []byte(settings), 0o644)).To(Succeed())
				Expect(os.WriteFile(hookPath, []byte("#!/usr/bin/env sh\nexit 0\n"), 0o755)).To(Succeed())
			},
			assert: func(ws lifecycleWorkspace) {
				settingsPath := filepath.Join(ws.home, ".codebuddy", "settings.json")
				hookPath := filepath.Join(ws.home, ".codebuddy", "hooks", "ccp-rewrite.sh")
				b, err := os.ReadFile(settingsPath)
				Expect(err).NotTo(HaveOccurred())

				got := string(b)
				Expect(got).To(ContainSubstring(`"theme": "light"`))
				Expect(got).NotTo(ContainSubstring(strings.ReplaceAll(hookPath, "\\", "\\\\")))
			},
		}),
		Entry("cursor preserves other files and directories", preserveOtherFileCase("cursor", "root", func(ws lifecycleWorkspace) string {
			Expect(os.MkdirAll(filepath.Join(ws.root, ".cursor", "rules"), 0o755)).To(Succeed())
			return filepath.Join(ws.root, ".cursor", "rules", "team.mdc")
		}, func(ws lifecycleWorkspace) string {
			return filepath.Join(ws.root, ".cursor", "rules")
		}, func(ws lifecycleWorkspace) { Expect(RunInit(toolsArgs("cursor"))).To(Succeed()) })),
		Entry("amazon-q preserves other files and directories", preserveOtherFileCase("amazon-q", "root", func(ws lifecycleWorkspace) string {
			Expect(os.MkdirAll(filepath.Join(ws.root, ".amazonq", "rules"), 0o755)).To(Succeed())
			return filepath.Join(ws.root, ".amazonq", "rules", "team.md")
		}, func(ws lifecycleWorkspace) string {
			return filepath.Join(ws.root, ".amazonq", "rules")
		}, func(ws lifecycleWorkspace) { Expect(RunInit(toolsArgs("amazon-q"))).To(Succeed()) })),
		Entry("trae preserves other files and directories", preserveOtherFileCase("trae", "root", func(ws lifecycleWorkspace) string {
			Expect(os.MkdirAll(filepath.Join(ws.root, ".trae", "rules"), 0o755)).To(Succeed())
			return filepath.Join(ws.root, ".trae", "rules", "team.md")
		}, func(ws lifecycleWorkspace) string {
			return filepath.Join(ws.root, ".trae", "rules")
		}, func(ws lifecycleWorkspace) { Expect(RunInit(toolsArgs("trae"))).To(Succeed()) })),
		Entry("kiro preserves other files and directories", preserveOtherFileCase("kiro", "root", func(ws lifecycleWorkspace) string {
			Expect(os.MkdirAll(filepath.Join(ws.home, ".kiro", "steering"), 0o755)).To(Succeed())
			return filepath.Join(ws.home, ".kiro", "steering", "team.md")
		}, func(ws lifecycleWorkspace) string {
			return filepath.Join(ws.home, ".kiro", "steering")
		}, func(ws lifecycleWorkspace) { Expect(RunInit(toolsArgs("kiro"))).To(Succeed()) })),
		Entry("kilocode preserves other files and directories", preserveOtherFileCase("kilocode", "root", func(ws lifecycleWorkspace) string {
			Expect(os.MkdirAll(filepath.Join(ws.home, ".config", "kilocode", "plugins"), 0o755)).To(Succeed())
			return filepath.Join(ws.home, ".config", "kilocode", "plugins", "team.js")
		}, func(ws lifecycleWorkspace) string {
			return filepath.Join(ws.home, ".config", "kilocode", "plugins")
		}, func(ws lifecycleWorkspace) { Expect(RunInit(toolsArgs("kilocode"))).To(Succeed()) })),
		Entry("roocode preserves other files and directories", preserveOtherFileCase("roocode", "root", func(ws lifecycleWorkspace) string {
			Expect(os.MkdirAll(filepath.Join(ws.home, ".roo", "rules"), 0o755)).To(Succeed())
			return filepath.Join(ws.home, ".roo", "rules", "team.md")
		}, func(ws lifecycleWorkspace) string {
			return filepath.Join(ws.home, ".roo", "rules")
		}, func(ws lifecycleWorkspace) { Expect(RunInit(toolsArgs("roocode"))).To(Succeed()) })),
	)

	It("reports noop, removed, and failed adapter uninstall states", func() {
		ctx := agents.Context{ScopeRoot: GinkgoT().TempDir(), HomeDir: GinkgoT().TempDir()}
		adapters := map[string]agents.Adapter{
			"noop":  uninstallStubAdapter{id: "noop"},
			"gone":  uninstallCapableAdapter{uninstallStubAdapter: uninstallStubAdapter{id: "gone"}, res: agents.InstallResult{Applied: 1}},
			"error": uninstallCapableAdapter{uninstallStubAdapter: uninstallStubAdapter{id: "error"}, err: errors.New("boom")},
		}

		states, err := applyUninstallAdapters(ctx, []string{"noop", "gone"}, adapters)
		Expect(err).NotTo(HaveOccurred())
		Expect(states).To(HaveLen(2))
		Expect(states[0].Status).To(Equal("noop"))
		Expect(states[1].Status).To(Equal("removed"))

		states, err = applyUninstallAdapters(ctx, []string{"error"}, adapters)
		Expect(err).To(HaveOccurred())
		Expect(states).To(HaveLen(1))
		Expect(states[0].Status).To(Equal("failed"))
	})

	It("joins adapter ids", func() {
		adapters := map[string]agents.Adapter{
			"beta":  uninstallStubAdapter{id: "beta"},
			"alpha": uninstallStubAdapter{id: "alpha"},
		}
		Expect(joinTools(adapters)).To(Equal("alpha, beta"))
	})

	DescribeTable("detecting tools automatically on uninstall",
		func(tc uninstallAutodetectCase) {
			ws := newUninstallWorkspace(tc.scope)
			for _, dir := range tc.setupDirs {
				Expect(os.MkdirAll(expandUninstallPath(ws, dir), 0o755)).To(Succeed())
			}

			Expect(RunInit(toolsArgs(tc.tool))).To(Succeed())
			Expect(RunUninstall(nil)).To(Succeed())
			expectMissingPath(expandUninstallPath(ws, tc.removed))
		},
		Entry("opencode", uninstallAutodetectCase{name: "opencode", tool: "opencode", scope: "root", setupDirs: []string{"{root}/.opencode"}, removed: "{home}/.config/opencode/plugins/ccp-rewrite.js"}),
		Entry("github-copilot", uninstallAutodetectCase{name: "github-copilot", tool: "github-copilot", scope: "root", setupDirs: []string{"{root}/.github"}, removed: "{home}/.copilot/copilot-instructions.md"}),
		Entry("gemini", uninstallAutodetectCase{name: "gemini", tool: "gemini", scope: "root", setupDirs: []string{"{root}/.gemini"}, removed: "{home}/.gemini/GEMINI.md"}),
		Entry("cursor", uninstallAutodetectCase{name: "cursor", tool: "cursor", scope: "root", setupDirs: []string{"{root}/.cursor"}, removed: "{root}/.cursor/rules/ccp.mdc"}),
		Entry("amazon-q", uninstallAutodetectCase{name: "amazon-q", tool: "amazon-q", scope: "root", setupDirs: []string{"{root}/.amazonq"}, removed: "{root}/.amazonq/rules/ccp.md"}),
		Entry("roocode", uninstallAutodetectCase{name: "roocode", tool: "roocode", scope: "root", setupDirs: []string{"{root}/.roo"}, removed: "{home}/.roo/rules/ccp.md"}),
	)

	It("returns an error when no tools are detected", func() {
		_ = newUninstallWorkspace("root")
		err := RunUninstall(nil)
		Expect(err).To(MatchError(ContainSubstring("no tools detected")))
	})

	It("uses configured tools when the flag is omitted", func() {
		ws := newUninstallWorkspace("root")
		Expect(os.MkdirAll(filepath.Join(ws.root, ".opencode"), 0o755)).To(Succeed())
		Expect(RunInit(toolsArgs("opencode"))).To(Succeed())
		Expect(RunUninstall(nil)).To(Succeed())
		expectMissingPath(filepath.Join(ws.home, ".config", "opencode", "plugins", "ccp-rewrite.js"))
	})
})

func toolsArgs(tool string) []string {
	return []string{toolsFlag, tool}
}

func newUninstallWorkspace(scope string) lifecycleWorkspace {
	ws := newLifecycleWorkspaceSpec()
	if scope == "root" {
		withWorkingDir(ws.root)
		return ws
	}
	withWorkingDir(ws.work)
	return ws
}

func expandUninstallPath(ws lifecycleWorkspace, value string) string {
	replaced := strings.NewReplacer(
		"{home}", ws.home,
		"{root}", ws.root,
		"{work}", ws.work,
	).Replace(value)
	return filepath.FromSlash(replaced)
}

func expectMissingPath(path string) {
	_, err := os.Stat(path)
	Expect(err).To(MatchError(os.ErrNotExist))
}

func expectDir(path string) {
	st, err := os.Stat(path)
	Expect(err).NotTo(HaveOccurred())
	Expect(st.IsDir()).To(BeTrue())
}

func existingPath(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func blockPreserveCase(tool string, scope string, pathFn func(lifecycleWorkspace) string) uninstallPreserveCase {
	return uninstallPreserveCase{
		name:  tool + " preserves non-CCP content",
		tool:  tool,
		scope: scope,
		setup: func(ws lifecycleWorkspace) {
			path := pathFn(ws)
			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			Expect(os.WriteFile(path, []byte("# User Content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nmanaged content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"), 0o644)).To(Succeed())
		},
		assert: func(ws lifecycleWorkspace) {
			path := pathFn(ws)
			b, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())

			got := string(b)
			Expect(got).NotTo(ContainSubstring("managed content"))
			Expect(got).To(ContainSubstring("# User Content"))
			Expect(got).To(ContainSubstring("# Tail"))
		},
	}
}

func preserveOtherFileCase(tool string, scope string, fileFn func(lifecycleWorkspace) string, dirFn func(lifecycleWorkspace) string, initFn func(lifecycleWorkspace)) uninstallPreserveCase {
	return uninstallPreserveCase{
		name:  tool + " preserves other files",
		tool:  tool,
		scope: scope,
		setup: func(ws lifecycleWorkspace) {
			otherFile := fileFn(ws)
			Expect(os.MkdirAll(filepath.Dir(otherFile), 0o755)).To(Succeed())
			Expect(os.WriteFile(otherFile, []byte("team rule\n"), 0o644)).To(Succeed())
			initFn(ws)
		},
		assert: func(ws lifecycleWorkspace) {
			Expect(existingPath(fileFn(ws))).To(BeTrue())
			expectDir(dirFn(ws))
		},
	}
}

type uninstallStubAdapter struct {
	id string
}

func (a uninstallStubAdapter) ID() string { return a.id }

func (a uninstallStubAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, a.id)
}

func (a uninstallStubAdapter) Install(_ agents.Context, _ agents.WriterFunc) (agents.InstallResult, error) {
	return agents.InstallResult{}, nil
}

func (a uninstallStubAdapter) Plan(_ agents.Context) []agents.PlannedArtifact {
	return nil
}

func (a uninstallStubAdapter) Verify(_ agents.Context) error {
	return nil
}

type uninstallCapableAdapter struct {
	uninstallStubAdapter
	res agents.InstallResult
	err error
}

func (a uninstallCapableAdapter) Uninstall(_ agents.Context) (agents.InstallResult, error) {
	return a.res, a.err
}
