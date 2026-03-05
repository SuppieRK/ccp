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
	initMkdirHomeErrFmt   = "mkdir home: %v"
	initOpenCodeDir       = ".opencode"
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

func TestRunInitLocalIdempotentAndBackup(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)

	if err := RunInit([]string{initToolsFlag, "cursor,opencode"}); err != nil {
		t.Fatalf(initFirstFailedFmt, err)
	}
	path := filepath.Join(tmp, ".ccp", initConfigFileName)
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

func TestRunInitLocalAppendsDotCCPToExistingGitignore(t *testing.T) {
	tmp := t.TempDir()
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
	if got != "node_modules\n.ccp\n" {
		t.Fatalf("unexpected .gitignore content: %q", got)
	}
}

func TestRunInitLocalDoesNotDuplicateDotCCPInGitignore(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)

	if err := os.WriteFile(initGitignoreName, []byte(".ccp\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	if err := RunInit([]string{initToolsFlag, "opencode"}); err != nil {
		t.Fatalf(initFailedFmt, err)
	}
	if err := RunInit([]string{initToolsFlag, "opencode,cursor"}); err != nil {
		t.Fatalf(initSecondFailedFmt, err)
	}

	b, err := os.ReadFile(initGitignoreName)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	got := string(b)
	if got != ".ccp\n" {
		t.Fatalf("expected single .ccp line, got: %q", got)
	}
}

func TestRunInitLocalSkipsGitignoreWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)

	if err := RunInit([]string{initToolsFlag, "opencode"}); err != nil {
		t.Fatalf(initFailedFmt, err)
	}
	if _, err := os.Stat(initGitignoreName); !os.IsNotExist(err) {
		t.Fatalf("expected .gitignore to remain absent, got err=%v", err)
	}
}

func TestRunInitGlobalWritesConfigUnderHome(t *testing.T) {
	tmp := t.TempDir()
	setHomeDirForTest(t, tmp)

	if err := RunInit([]string{"--global", initToolsFlag, "cursor,opencode"}); err != nil {
		t.Fatalf("global init failed: %v", err)
	}

	path := filepath.Join(tmp, ".config", "ccp", initConfigFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read global init config: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal global init config: %v", err)
	}
	if got, _ := cfg["scope"].(string); got != "global" {
		t.Fatalf("scope = %q, want global", got)
	}
}

func TestRunInitPersistsStateShape(t *testing.T) {
	tmp := t.TempDir()
	setHomeDirForTest(t, tmp)
	chdirForTest(t, tmp)

	if err := RunInit([]string{initToolsFlag, "cursor"}); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	path := filepath.Join(tmp, ".ccp", initConfigFileName)
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
	if cfg.State[0].Tool != "cursor" || cfg.State[0].Status != "noop" {
		t.Fatalf("unexpected state entry: %+v", cfg.State[0])
	}
	if !strings.Contains(cfg.State[0].Reason, "applied=0 noop=1") {
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

	path := filepath.Join(tmp, ".ccp", initConfigFileName)
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
	if _, err := os.Stat(filepath.Join(tmp, initOpenCodeDir, "plugins", initOpenCodeRewriteJS)); err != nil {
		t.Fatalf("expected opencode plugin to be installed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, initCodexDir, initAgentsFileName)); !os.IsNotExist(err) {
		t.Fatalf("did not expect codex global agents file to be created, err=%v", err)
	}
}

func TestRunInitOpencodeInstallsLocalPlugin(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)
	if err := RunInit([]string{initToolsFlag, "opencode"}); err != nil {
		t.Fatalf("opencode init failed: %v", err)
	}
	pluginPath := filepath.Join(tmp, initOpenCodeDir, "plugins", initOpenCodeRewriteJS)
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
}

func TestRunInitOpencodeGlobalInstallsPluginUnderConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdirAllForTest(t, home, initMkdirHomeErrFmt)
	setHomeDirForTest(t, home)

	if err := RunInit([]string{"--global", initToolsFlag, "opencode"}); err != nil {
		t.Fatalf("global opencode init failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode", "plugins", initOpenCodeRewriteJS)); err != nil {
		t.Fatalf("expected global opencode plugin, err=%v", err)
	}
}

func TestRunInitOpencodeIdempotentRerun(t *testing.T) {
	tmp := t.TempDir()
	chdirForTest(t, tmp)

	if err := RunInit([]string{initToolsFlag, "opencode"}); err != nil {
		t.Fatalf(initFirstFailedFmt, err)
	}
	pluginPath := filepath.Join(tmp, initOpenCodeDir, "plugins", initOpenCodeRewriteJS)
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
