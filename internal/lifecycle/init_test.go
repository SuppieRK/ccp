package lifecycle

import (
	"encoding/json"
	"go-command-compression-proxy/internal/lifecycle/agents"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	initChdirErrFmt       = "chdir: %v"
	initToolsFlag         = "--tools"
	initConfigFileName    = "init.json"
	initFirstFailedFmt    = "first init failed: %v"
	initFailedFmt         = "init failed: %v"
	initSecondFailedFmt   = "second init failed: %v"
	initGitignoreName     = ".gitignore"
	initCodexDir          = ".codex"
	initCopilotDir        = ".copilot"
	initCopilotFileName   = "copilot-instructions.md"
	initCursorDir         = ".cursor"
	initCursorRuleName    = "ccp.mdc"
	initAmazonQDir        = ".amazonq"
	initAmazonQRuleName   = "ccp.md"
	initGeminiDir         = ".gemini"
	initGeminiFileName    = "GEMINI.md"
	initWindsurfDir       = ".windsurf"
	initWindsurfRuleName  = "ccp.md"
	initClineDir          = ".clinerules"
	initClineRuleName     = "ccp.md"
	initMkdirHomeErrFmt   = "mkdir home: %v"
	initOpenCodeRewriteJS = "ccp-rewrite.js"
	initAgentsFileName    = "AGENTS.md"
	initMkdirWorkErrFmt   = "mkdir work: %v"
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

func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	oldWd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf(initChdirErrFmt, err)
	}
}

func mkdirAllForTest(t *testing.T, path, errFmt string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf(errFmt, err)
	}
}

func TestRunInitGlobalConfigIdempotentAndBackup(t *testing.T) {
	tmp := t.TempDir()
	setHomeDirForTest(t, tmp)
	chdirForTest(t, tmp)

	if err := RunInit([]string{initToolsFlag, "cursor,opencode"}); err != nil {
		t.Fatalf(initFirstFailedFmt, err)
	}
	path := filepath.Join(tmp, ".config", "ccp", initConfigFileName)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("init file missing: %v", err)
	}

	if err := RunInit([]string{initToolsFlag, "opencode,cursor"}); err != nil {
		t.Fatalf("idempotent init failed: %v", err)
	}

	if err := RunInit([]string{initToolsFlag, "opencode"}); err != nil {
		t.Fatalf("changed init failed: %v", err)
	}
	matches, _ := filepath.Glob(path + ".bak.*")
	if len(matches) == 0 {
		t.Fatal("expected backup file when config changes")
	}
}

func TestRunInitDoesNotModifyGitignore(t *testing.T) {
	tmp := t.TempDir()
	setHomeDirForTest(t, tmp)
	chdirForTest(t, tmp)

	if err := os.WriteFile(initGitignoreName, []byte("node_modules\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	if err := RunInit([]string{initToolsFlag, "opencode"}); err != nil {
		t.Fatalf(initFailedFmt, err)
	}

	b, err := os.ReadFile(initGitignoreName)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	got := string(b)
	if got != "node_modules\n" {
		t.Fatalf("unexpected .gitignore content: %q", got)
	}
}

func TestRunInitSkipsGitignoreWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	setHomeDirForTest(t, tmp)
	chdirForTest(t, tmp)

	if err := RunInit([]string{initToolsFlag, "opencode"}); err != nil {
		t.Fatalf(initFailedFmt, err)
	}
	if _, err := os.Stat(initGitignoreName); !os.IsNotExist(err) {
		t.Fatalf("expected .gitignore to remain absent, got err=%v", err)
	}
}

func TestRunInitWritesConfigUnderHome(t *testing.T) {
	ws := newLifecycleWorkspace(t)

	if err := RunInit([]string{initToolsFlag, "cursor,opencode"}); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	path := filepath.Join(ws.home, ".config", "ccp", initConfigFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read global init config: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal init config: %v", err)
	}
	if _, ok := cfg["scope"]; ok {
		t.Fatalf("did not expect deprecated scope field in config: %v", cfg)
	}
}

func TestRunInitPersistsStateShape(t *testing.T) {
	tmp := t.TempDir()
	setHomeDirForTest(t, tmp)
	chdirForTest(t, tmp)

	if err := RunInit([]string{initToolsFlag, "cursor"}); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	path := filepath.Join(tmp, ".config", "ccp", initConfigFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read init config: %v", err)
	}

	var cfg initConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal init config: %v", err)
	}
	if len(cfg.State) != 1 {
		t.Fatalf("state len = %d, want 1", len(cfg.State))
	}
	if cfg.State[0].Tool != "cursor" || cfg.State[0].Status != "applied" {
		t.Fatalf("unexpected state entry: %+v", cfg.State[0])
	}
	if !strings.Contains(cfg.State[0].Reason, "applied=1 noop=0") {
		t.Fatalf("unexpected state reason: %q", cfg.State[0].Reason)
	}
}

func TestRunInitDetectsToolsWhenMissingToolsFlag(t *testing.T) {
	tmp := t.TempDir()
	setHomeDirForTest(t, tmp)
	chdirForTest(t, tmp)
	mkdirAllForTest(t, initCodexDir, "mkdir .codex: %v")

	if err := RunInit([]string{}); err != nil {
		t.Fatalf("detected init failed: %v", err)
	}

	path := filepath.Join(tmp, ".config", "ccp", initConfigFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read init config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal init config: %v", err)
	}
	tools, _ := cfg["tools"].([]any)
	if len(tools) != 1 || tools[0] != "codex" {
		t.Fatalf("tools = %v, want [codex]", tools)
	}
	if _, err := os.Stat(filepath.Join(tmp, initCodexDir, initAgentsFileName)); err != nil {
		t.Fatalf("expected home-scoped codex agents file after detection, err=%v", err)
	}
}

func TestRunInitDetectsGitHubCopilotWhenMissingToolsFlag(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)
	chdirForTest(t, tmp)
	mkdirAllForTest(t, ".github", "mkdir .github: %v")

	if err := RunInit(nil); err != nil {
		t.Fatalf("detected init failed: %v", err)
	}

	path := filepath.Join(home, ".config", "ccp", initConfigFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read init config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal init config: %v", err)
	}
	tools, _ := cfg["tools"].([]any)
	if len(tools) != 1 || tools[0] != "github-copilot" {
		t.Fatalf("tools = %v, want [github-copilot]", tools)
	}
	if _, err := os.Stat(filepath.Join(home, initCopilotDir, initCopilotFileName)); err != nil {
		t.Fatalf("expected github copilot instructions file after detection, err=%v", err)
	}
}

func TestRunInitDetectsCursorWhenMissingToolsFlag(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)
	chdirForTest(t, tmp)
	mkdirAllForTest(t, initCursorDir, "mkdir .cursor: %v")

	if err := RunInit(nil); err != nil {
		t.Fatalf("detected init failed: %v", err)
	}

	path := filepath.Join(home, ".config", "ccp", initConfigFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read init config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal init config: %v", err)
	}
	tools, _ := cfg["tools"].([]any)
	if len(tools) != 1 || tools[0] != "cursor" {
		t.Fatalf("tools = %v, want [cursor]", tools)
	}
	if _, err := os.Stat(filepath.Join(tmp, initCursorDir, "rules", initCursorRuleName)); err != nil {
		t.Fatalf("expected cursor rule file after detection, err=%v", err)
	}
}

func TestRunInitDetectsGeminiWhenMissingToolsFlag(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)
	chdirForTest(t, tmp)
	mkdirAllForTest(t, initGeminiDir, "mkdir .gemini: %v")

	if err := RunInit(nil); err != nil {
		t.Fatalf("detected init failed: %v", err)
	}

	path := filepath.Join(home, ".config", "ccp", initConfigFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read init config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal init config: %v", err)
	}
	tools, _ := cfg["tools"].([]any)
	if len(tools) != 1 || tools[0] != "gemini" {
		t.Fatalf("tools = %v, want [gemini]", tools)
	}
	if _, err := os.Stat(filepath.Join(home, initGeminiDir, initGeminiFileName)); err != nil {
		t.Fatalf("expected gemini instructions file after detection, err=%v", err)
	}
}

func TestRunInitDetectsAmazonQWhenMissingToolsFlag(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)
	chdirForTest(t, tmp)
	mkdirAllForTest(t, initAmazonQDir, "mkdir .amazonq: %v")

	if err := RunInit(nil); err != nil {
		t.Fatalf("detected init failed: %v", err)
	}

	path := filepath.Join(home, ".config", "ccp", initConfigFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read init config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal init config: %v", err)
	}
	tools, _ := cfg["tools"].([]any)
	if len(tools) != 1 || tools[0] != "amazon-q" {
		t.Fatalf("tools = %v, want [amazon-q]", tools)
	}
	if _, err := os.Stat(filepath.Join(tmp, initAmazonQDir, "rules", initAmazonQRuleName)); err != nil {
		t.Fatalf("expected amazon q rule file after detection, err=%v", err)
	}
}

func TestRunInitDetectsWindsurfWhenMissingToolsFlag(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)
	chdirForTest(t, tmp)
	mkdirAllForTest(t, initWindsurfDir, "mkdir .windsurf: %v")

	if err := RunInit(nil); err != nil {
		t.Fatalf("detected init failed: %v", err)
	}

	path := filepath.Join(home, ".config", "ccp", initConfigFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read init config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal init config: %v", err)
	}
	tools, _ := cfg["tools"].([]any)
	if len(tools) != 1 || tools[0] != "windsurf" {
		t.Fatalf("tools = %v, want [windsurf]", tools)
	}
	if _, err := os.Stat(filepath.Join(tmp, initWindsurfDir, "rules", initWindsurfRuleName)); err != nil {
		t.Fatalf("expected windsurf rule file after detection, err=%v", err)
	}
}

func TestRunInitDetectsClineWhenMissingToolsFlag(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)
	chdirForTest(t, tmp)
	mkdirAllForTest(t, initClineDir, "mkdir .clinerules: %v")

	if err := RunInit(nil); err != nil {
		t.Fatalf("detected init failed: %v", err)
	}

	path := filepath.Join(home, ".config", "ccp", initConfigFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read init config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal init config: %v", err)
	}
	tools, _ := cfg["tools"].([]any)
	if len(tools) != 1 || tools[0] != "cline" {
		t.Fatalf("tools = %v, want [cline]", tools)
	}
	if _, err := os.Stat(filepath.Join(tmp, initClineDir, initClineRuleName)); err != nil {
		t.Fatalf("expected cline rule file after detection, err=%v", err)
	}
}

func TestRunInitExplicitToolsOverrideDetected(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)
	chdirForTest(t, tmp)
	mkdirAllForTest(t, initCodexDir, "mkdir .codex: %v")

	if err := RunInit([]string{initToolsFlag, "opencode"}); err != nil {
		t.Fatalf("explicit init failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode", "plugins", initOpenCodeRewriteJS)); err != nil {
		t.Fatalf("expected opencode plugin to be installed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, initCodexDir, initAgentsFileName)); !os.IsNotExist(err) {
		t.Fatalf("did not expect codex global agents file to be created, err=%v", err)
	}
}

func TestRunInitOpencodeInstallsGlobalPlugin(t *testing.T) {
	tmp := t.TempDir()
	setHomeDirForTest(t, tmp)
	chdirForTest(t, tmp)
	if err := RunInit([]string{initToolsFlag, "opencode"}); err != nil {
		t.Fatalf("opencode init failed: %v", err)
	}
	pluginPath := filepath.Join(tmp, ".config", "opencode", "plugins", initOpenCodeRewriteJS)
	b, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("read opencode plugin: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"tool.execute.before"`) {
		t.Fatalf("expected tool.execute.before hook in plugin, got: %s", s)
	}
	if !strings.Contains(s, `input.tool !== "bash"`) {
		t.Fatalf("expected bash-only guard in plugin, got: %s", s)
	}
	if !strings.Contains(s, `trimmed.startsWith("ccp ")`) {
		t.Fatalf("expected ccp prefix guard in plugin, got: %s", s)
	}
	if !strings.Contains(s, `trimmed === "ccp"`) {
		t.Fatalf("expected bare ccp guard in plugin, got: %s", s)
	}
	if !strings.Contains(s, `if (/['"\\]|\$\(|\$\{|<</.test(command))`) {
		t.Fatalf("expected conservative complexity fallback guard in plugin, got: %s", s)
	}
	if !strings.Contains(s, `command.replace(/(^|\|\||&&|\||;)\s*(?!ccp\b)/g, "$1 ccp ")`) {
		t.Fatalf("expected chained-segment rewrite rule in plugin, got: %s", s)
	}
	if !strings.Contains(s, `output.args.command = rewritten;`) {
		t.Fatalf("expected rewritten command assignment in plugin, got: %s", s)
	}
}

func TestRunInitOpencodeIdempotentRerun(t *testing.T) {
	tmp := t.TempDir()
	setHomeDirForTest(t, tmp)
	chdirForTest(t, tmp)

	if err := RunInit([]string{initToolsFlag, "opencode"}); err != nil {
		t.Fatalf(initFirstFailedFmt, err)
	}
	pluginPath := filepath.Join(tmp, ".config", "opencode", "plugins", initOpenCodeRewriteJS)
	infoBefore, err := os.Stat(pluginPath)
	if err != nil {
		t.Fatalf("stat plugin before rerun: %v", err)
	}
	beforeData, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("read plugin before rerun: %v", err)
	}

	if err := RunInit([]string{initToolsFlag, "opencode"}); err != nil {
		t.Fatalf(initSecondFailedFmt, err)
	}
	infoAfter, err := os.Stat(pluginPath)
	if err != nil {
		t.Fatalf("stat plugin after rerun: %v", err)
	}
	afterData, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("read plugin after rerun: %v", err)
	}

	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatalf("expected idempotent rerun to keep plugin timestamp, before=%v after=%v", infoBefore.ModTime(), infoAfter.ModTime())
	}
	if string(afterData) != string(beforeData) {
		t.Fatalf("expected idempotent rerun to keep plugin content unchanged")
	}
	if matches, _ := filepath.Glob(pluginPath + ".bak.*"); len(matches) != 0 {
		t.Fatalf("expected no plugin backups for idempotent rerun, got %d", len(matches))
	}
}

func TestRunInitCodexUsesGlobalAgentsManagedBlock(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, initMkdirWorkErrFmt)
	chdirForTest(t, work)

	if err := RunInit([]string{initToolsFlag, "codex"}); err != nil {
		t.Fatalf("codex init failed: %v", err)
	}
	agentsPath := filepath.Join(home, initCodexDir, initAgentsFileName)
	b, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read codex agents file: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "<!-- BEGIN: CCP MANAGED BLOCK -->") || !strings.Contains(s, "<!-- END: CCP MANAGED BLOCK -->") {
		t.Fatalf("expected managed markers, got: %s", s)
	}
	if !strings.Contains(s, "Use `ccp` as the command prefix for every executable in shell commands, including chained (`&&`, `||`) and piped (`|`) expressions.") {
		t.Fatalf("expected preferred ccp wording, got: %s", s)
	}
	if !strings.Contains(s, "`ccp ls -la`") {
		t.Fatalf("expected canonical examples, got: %s", s)
	}
	if !strings.Contains(s, "`ccp echo chain-ok && ccp echo chain-done`") {
		t.Fatalf("expected chaining examples, got: %s", s)
	}
	if !strings.Contains(s, "`ccp nl -ba spec.md | ccp sed -n '1,260p'`") {
		t.Fatalf("expected tested pipeline example, got: %s", s)
	}
	if !strings.Contains(s, "If `ccp` is unavailable, run the original command and note that CCP is not installed.") {
		t.Fatalf("expected fallback wording, got: %s", s)
	}
	if _, err := os.Stat(filepath.Join(home, initCodexDir, "hooks", initRewriteScriptName)); !os.IsNotExist(err) {
		t.Fatalf("did not expect codex hook script path, err=%v", err)
	}
}

func TestRunInitCodexRerunDoesNotDuplicateManagedBlock(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, initMkdirWorkErrFmt)
	chdirForTest(t, work)

	if err := RunInit([]string{initToolsFlag, "codex"}); err != nil {
		t.Fatalf(initFirstFailedFmt, err)
	}
	agentsPath := filepath.Join(home, initCodexDir, initAgentsFileName)
	before, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	if err := RunInit([]string{initToolsFlag, "codex"}); err != nil {
		t.Fatalf(initSecondFailedFmt, err)
	}
	after, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("expected idempotent rerun to keep file unchanged")
	}
	if strings.Count(string(after), "<!-- BEGIN: CCP MANAGED BLOCK -->") != 1 {
		t.Fatalf("expected single begin marker")
	}
	if strings.Count(string(after), "<!-- END: CCP MANAGED BLOCK -->") != 1 {
		t.Fatalf("expected single end marker")
	}
}

func TestRunInitCodexPreservesUserContentAndReplacesOnlyManagedRegion(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, filepath.Join(home, initCodexDir), "mkdir codex home: %v")
	setHomeDirForTest(t, home)

	agentsPath := filepath.Join(home, initCodexDir, initAgentsFileName)
	initial := "# User Header\n\ncustom content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nold content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"
	if err := os.WriteFile(agentsPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial agents file: %v", err)
	}

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, initMkdirWorkErrFmt)
	chdirForTest(t, work)

	if err := RunInit([]string{initToolsFlag, "codex"}); err != nil {
		t.Fatalf("codex init failed: %v", err)
	}
	updated, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read updated agents file: %v", err)
	}
	s := string(updated)
	if !strings.Contains(s, "# User Header") || !strings.Contains(s, "# Tail") {
		t.Fatalf("expected user-authored content to be preserved, got: %s", s)
	}
	if strings.Contains(s, "old content") {
		t.Fatalf("expected old managed content to be replaced, got: %s", s)
	}
}

func TestRunInitGitHubCopilotUsesUserInstructionsManagedBlock(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, initMkdirWorkErrFmt)
	chdirForTest(t, work)

	if err := RunInit([]string{initToolsFlag, "github-copilot"}); err != nil {
		t.Fatalf("github copilot init failed: %v", err)
	}
	instructionsPath := filepath.Join(home, initCopilotDir, initCopilotFileName)
	b, err := os.ReadFile(instructionsPath)
	if err != nil {
		t.Fatalf("read github copilot instructions file: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "<!-- BEGIN: CCP MANAGED BLOCK -->") || !strings.Contains(s, "<!-- END: CCP MANAGED BLOCK -->") {
		t.Fatalf("expected managed markers, got: %s", s)
	}
	if !strings.Contains(s, "Use `ccp` as the command prefix for every executable in shell commands, including chained (`&&`, `||`) and piped (`|`) expressions.") {
		t.Fatalf("expected preferred ccp wording, got: %s", s)
	}
}

func TestRunInitGitHubCopilotRerunDoesNotDuplicateManagedBlock(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, initMkdirWorkErrFmt)
	chdirForTest(t, work)

	if err := RunInit([]string{initToolsFlag, "github-copilot"}); err != nil {
		t.Fatalf(initFirstFailedFmt, err)
	}
	instructionsPath := filepath.Join(home, initCopilotDir, initCopilotFileName)
	before, err := os.ReadFile(instructionsPath)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	if err := RunInit([]string{initToolsFlag, "github-copilot"}); err != nil {
		t.Fatalf(initSecondFailedFmt, err)
	}
	after, err := os.ReadFile(instructionsPath)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("expected idempotent rerun to keep file unchanged")
	}
	if strings.Count(string(after), "<!-- BEGIN: CCP MANAGED BLOCK -->") != 1 {
		t.Fatalf("expected single begin marker")
	}
	if strings.Count(string(after), "<!-- END: CCP MANAGED BLOCK -->") != 1 {
		t.Fatalf("expected single end marker")
	}
}

func TestRunInitGitHubCopilotPreservesUserContentAndReplacesOnlyManagedRegion(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, filepath.Join(home, initCopilotDir), "mkdir copilot home: %v")
	setHomeDirForTest(t, home)

	instructionsPath := filepath.Join(home, initCopilotDir, initCopilotFileName)
	initial := "# User Header\n\ncustom content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nold content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"
	if err := os.WriteFile(instructionsPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial instructions file: %v", err)
	}

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, initMkdirWorkErrFmt)
	chdirForTest(t, work)

	if err := RunInit([]string{initToolsFlag, "github-copilot"}); err != nil {
		t.Fatalf("github copilot init failed: %v", err)
	}
	updated, err := os.ReadFile(instructionsPath)
	if err != nil {
		t.Fatalf("read updated instructions file: %v", err)
	}
	s := string(updated)
	if !strings.Contains(s, "# User Header") || !strings.Contains(s, "# Tail") {
		t.Fatalf("expected user-authored content to be preserved, got: %s", s)
	}
	if strings.Contains(s, "old content") {
		t.Fatalf("expected old managed content to be replaced, got: %s", s)
	}
}

func TestRunInitGeminiUsesUserInstructionsManagedBlock(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, initMkdirWorkErrFmt)
	chdirForTest(t, work)

	if err := RunInit([]string{initToolsFlag, "gemini"}); err != nil {
		t.Fatalf("gemini init failed: %v", err)
	}
	instructionsPath := filepath.Join(home, initGeminiDir, initGeminiFileName)
	b, err := os.ReadFile(instructionsPath)
	if err != nil {
		t.Fatalf("read gemini instructions file: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "<!-- BEGIN: CCP MANAGED BLOCK -->") || !strings.Contains(s, "<!-- END: CCP MANAGED BLOCK -->") {
		t.Fatalf("expected managed markers, got: %s", s)
	}
	if !strings.Contains(s, "Use `ccp` as the command prefix for every executable in shell commands, including chained (`&&`, `||`) and piped (`|`) expressions.") {
		t.Fatalf("expected preferred ccp wording, got: %s", s)
	}
}

func TestRunInitGeminiRerunDoesNotDuplicateManagedBlock(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, initMkdirWorkErrFmt)
	chdirForTest(t, work)

	if err := RunInit([]string{initToolsFlag, "gemini"}); err != nil {
		t.Fatalf(initFirstFailedFmt, err)
	}
	instructionsPath := filepath.Join(home, initGeminiDir, initGeminiFileName)
	before, err := os.ReadFile(instructionsPath)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	if err := RunInit([]string{initToolsFlag, "gemini"}); err != nil {
		t.Fatalf(initSecondFailedFmt, err)
	}
	after, err := os.ReadFile(instructionsPath)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("expected idempotent rerun to keep file unchanged")
	}
	if strings.Count(string(after), "<!-- BEGIN: CCP MANAGED BLOCK -->") != 1 {
		t.Fatalf("expected single begin marker")
	}
	if strings.Count(string(after), "<!-- END: CCP MANAGED BLOCK -->") != 1 {
		t.Fatalf("expected single end marker")
	}
}

func TestRunInitGeminiPreservesUserContentAndReplacesOnlyManagedRegion(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, filepath.Join(home, initGeminiDir), "mkdir gemini home: %v")
	setHomeDirForTest(t, home)

	instructionsPath := filepath.Join(home, initGeminiDir, initGeminiFileName)
	initial := "# User Header\n\ncustom content\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nold content\n<!-- END: CCP MANAGED BLOCK -->\n\n# Tail\n"
	if err := os.WriteFile(instructionsPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial instructions file: %v", err)
	}

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, initMkdirWorkErrFmt)
	chdirForTest(t, work)

	if err := RunInit([]string{initToolsFlag, "gemini"}); err != nil {
		t.Fatalf("gemini init failed: %v", err)
	}
	updated, err := os.ReadFile(instructionsPath)
	if err != nil {
		t.Fatalf("read updated instructions file: %v", err)
	}
	s := string(updated)
	if !strings.Contains(s, "# User Header") || !strings.Contains(s, "# Tail") {
		t.Fatalf("expected user-authored content to be preserved, got: %s", s)
	}
	if strings.Contains(s, "old content") {
		t.Fatalf("expected old managed content to be replaced, got: %s", s)
	}
}

func TestRunInitCursorUsesManagedProjectRule(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, initCursorDir), "mkdir .cursor: %v")
	chdirForTest(t, tmp)

	if err := RunInit([]string{initToolsFlag, "cursor"}); err != nil {
		t.Fatalf("cursor init failed: %v", err)
	}
	rulePath := filepath.Join(tmp, initCursorDir, "rules", initCursorRuleName)
	b, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatalf("read cursor rule file: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "alwaysApply: true") {
		t.Fatalf("expected alwaysApply metadata, got: %s", s)
	}
	if !strings.Contains(s, "Use `ccp` as the command prefix for every executable in shell commands, including chained (`&&`, `||`) and piped (`|`) expressions.") {
		t.Fatalf("expected canonical ccp wording, got: %s", s)
	}
	if !strings.Contains(s, "`ccp echo chain-ok && ccp echo chain-done`") {
		t.Fatalf("expected chaining examples, got: %s", s)
	}
	if strings.Contains(s, "<!-- BEGIN: CCP MANAGED BLOCK -->") || strings.Contains(s, "<!-- END: CCP MANAGED BLOCK -->") {
		t.Fatalf("did not expect managed block markers in cursor rule, got: %s", s)
	}
}

func TestRunInitCursorRerunKeepsManagedRuleStable(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)
	mkdirAllForTest(t, filepath.Join(tmp, initCursorDir), "mkdir .cursor: %v")
	chdirForTest(t, tmp)

	if err := RunInit([]string{initToolsFlag, "cursor"}); err != nil {
		t.Fatalf(initFirstFailedFmt, err)
	}
	rulePath := filepath.Join(tmp, initCursorDir, "rules", initCursorRuleName)
	before, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	infoBefore, err := os.Stat(rulePath)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	if err := RunInit([]string{initToolsFlag, "cursor"}); err != nil {
		t.Fatalf(initSecondFailedFmt, err)
	}
	after, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	infoAfter, err := os.Stat(rulePath)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("expected idempotent rerun to keep rule content unchanged")
	}
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatalf("expected idempotent rerun to keep rule timestamp, before=%v after=%v", infoBefore.ModTime(), infoAfter.ModTime())
	}
	if matches, _ := filepath.Glob(rulePath + ".bak.*"); len(matches) != 0 {
		t.Fatalf("expected no rule backups for idempotent rerun, got %d", len(matches))
	}
}

func TestApplyAdaptersRoutesViaInstallerContract(t *testing.T) {
	fake := &fakeInstallAdapter{}
	scope := agents.Context{ScopeRoot: t.TempDir(), HomeDir: t.TempDir()}
	tools := []string{"fake"}
	adapters := map[string]agents.Adapter{"fake": fake}
	states, err := applyAdapters(scope, tools, adapters)
	if err != nil {
		t.Fatalf("applyAdapters failed: %v", err)
	}
	if fake.installed != 1 {
		t.Fatalf("expected installer to be called once, got %d", fake.installed)
	}
	if len(states) != 1 || states[0].Status != "applied" {
		t.Fatalf("unexpected states: %+v", states)
	}
}

func TestRunInitClaudeUsesHomeTargetsAndPreToolUseSettings(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, initMkdirWorkErrFmt)
	chdirForTest(t, work)

	if err := RunInit([]string{initToolsFlag, "claude"}); err != nil {
		t.Fatalf("claude init failed: %v", err)
	}

	hookPath := filepath.Join(home, initClaudeDir, "hooks", initRewriteScriptName)
	settingsPath := filepath.Join(home, initClaudeDir, initSettingsFileName)
	awarenessPath := filepath.Join(home, initClaudeDir, "CCP.md")

	hookInfo, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("missing claude hook: %v", err)
	}
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("missing claude settings: %v", err)
	}
	if _, err := os.Stat(awarenessPath); err != nil {
		t.Fatalf("missing claude awareness: %v", err)
	}
	if runtime.GOOS != "windows" && (hookInfo.Mode()&0o111) == 0 {
		t.Fatalf("expected claude hook to be executable, mode=%v", hookInfo.Mode())
	}
	if runtime.GOOS != "windows" {
		cmd := exec.Command("sh", "-n", hookPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("expected syntactically valid claude hook script, err=%v output=%s", err, string(out))
		}
	}

	b, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "\"PreToolUse\"") {
		t.Fatalf("expected PreToolUse registration, got: %s", s)
	}
	if !strings.Contains(s, "\"matcher\": \"Bash\"") {
		t.Fatalf("expected Bash matcher, got: %s", s)
	}
	if !strings.Contains(s, "\"command\": \""+strings.ReplaceAll(hookPath, "\\", "\\\\")+"\"") {
		t.Fatalf("expected hook command in settings, got: %s", s)
	}
}

func TestRunInitClaudeIdempotentRerun(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)

	work := filepath.Join(tmp, "work")
	mkdirAllForTest(t, work, initMkdirWorkErrFmt)
	chdirForTest(t, work)

	if err := RunInit([]string{initToolsFlag, "claude"}); err != nil {
		t.Fatalf(initFirstFailedFmt, err)
	}
	hookPath := filepath.Join(home, initClaudeDir, "hooks", initRewriteScriptName)
	settingsPath := filepath.Join(home, initClaudeDir, initSettingsFileName)
	awarenessPath := filepath.Join(home, initClaudeDir, "CCP.md")
	managedPaths := []string{hookPath, settingsPath, awarenessPath}

	beforeInfo := map[string]os.FileInfo{}
	beforeData := map[string][]byte{}
	for _, p := range managedPaths {
		info, statErr := os.Stat(p)
		if statErr != nil {
			t.Fatalf("stat managed artifact before rerun (%s): %v", p, statErr)
		}
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			t.Fatalf("read managed artifact before rerun (%s): %v", p, readErr)
		}
		beforeInfo[p] = info
		beforeData[p] = b
	}

	if err := RunInit([]string{initToolsFlag, "claude"}); err != nil {
		t.Fatalf(initSecondFailedFmt, err)
	}

	for _, p := range managedPaths {
		afterInfo, statErr := os.Stat(p)
		if statErr != nil {
			t.Fatalf("stat managed artifact after rerun (%s): %v", p, statErr)
		}
		afterData, readErr := os.ReadFile(p)
		if readErr != nil {
			t.Fatalf("read managed artifact after rerun (%s): %v", p, readErr)
		}
		if !afterInfo.ModTime().Equal(beforeInfo[p].ModTime()) {
			t.Fatalf("expected idempotent rerun to keep timestamp for %s, before=%v after=%v", p, beforeInfo[p].ModTime(), afterInfo.ModTime())
		}
		if string(afterData) != string(beforeData[p]) {
			t.Fatalf("expected idempotent rerun to keep content unchanged for %s", p)
		}
		if matches, _ := filepath.Glob(p + ".bak.*"); len(matches) != 0 {
			t.Fatalf("expected no backups for idempotent rerun at %s, got %d", p, len(matches))
		}
	}
}
