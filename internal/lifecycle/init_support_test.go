package lifecycle

import (
	"go-command-compression-proxy/internal/lifecycle/agents"
	"path/filepath"
)

const (
	initToolsFlag         = "--tools"
	initConfigFileName    = "init.json"
	initGitignoreName     = ".gitignore"
	initCodexDir          = ".codex"
	initCopilotDir        = ".copilot"
	initCopilotFileName   = "copilot-instructions.md"
	initClineDir          = ".clinerules"
	initClineRuleName     = "ccp.md"
	initRawEscapeHatch    = "If output seems corrupted, malformed, or unusable for the task, retry the command with `ccp --raw` as an escape hatch."
	initCursorDir         = ".cursor"
	initCursorRuleName    = "ccp.mdc"
	initAmazonQDir        = ".amazonq"
	initAmazonQRuleName   = "ccp.md"
	initAiderConfigName   = ".aider.conf.yml"
	initAuggieDir         = ".augment"
	initAntigravityDir    = ".agent"
	initCodeBuddyDir      = ".codebuddy"
	initCrushDir          = ".crush"
	initPiDir             = ".pi"
	initFactoryDir        = ".factory"
	initKiroDir           = ".kiro"
	initKiroRuleName      = "AGENTS.md"
	initKilocodeDir       = ".kilocode"
	initQoderDir          = ".qoder"
	initRooCodeDir        = ".roo"
	initRooCodeRuleName   = "ccp.md"
	initTraeDir           = ".trae"
	initTraeRuleName      = "ccp.md"
	initWindsurfDir       = ".windsurf"
	initWindsurfRuleName  = "ccp.md"
	initGeminiDir         = ".gemini"
	initGeminiFileName    = "GEMINI.md"
	initOpenCodeRewriteJS = "ccp-rewrite.js"
	initAgentsFileName    = "AGENTS.md"
	initQwenDir           = ".qwen"
	initRewriteScriptName = "ccp-rewrite.sh"
	initClaudeDir         = ".claude"
	initSettingsFileName  = "settings.json"
)

type fakeInstallAdapter struct {
	installed int
}

func (f *fakeInstallAdapter) ID() string { return "fake" }

func (f *fakeInstallAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".fake")
}

func (f *fakeInstallAdapter) Install(_ agents.Context, _ agents.WriterFunc) (agents.InstallResult, error) {
	f.installed++
	return agents.InstallResult{Applied: 1}, nil
}

func (f *fakeInstallAdapter) Plan(_ agents.Context) []agents.PlannedArtifact {
	return nil
}

func (f *fakeInstallAdapter) Verify(_ agents.Context) error { return nil }
