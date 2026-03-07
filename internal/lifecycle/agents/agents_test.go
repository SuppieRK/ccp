package agents

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type stubAdapter struct {
	id        string
	detectDir string
	plan      []PlannedArtifact
}

const (
	errInstallFmt          = "install error: %v"
	errUnexpectedIDFmt     = "unexpected id %q"
	errUnexpectedRootFmt   = "unexpected detect root %q"
	errUnexpectedInstFmt   = "unexpected install result %+v err=%v"
	errUnexpectedUninstFmt = "unexpected uninstall result %+v err=%v"
	errVerifyFmt           = "verify error: %v"
	agentsFileName         = "AGENTS.md"
)

func (s stubAdapter) ID() string {
	return s.id
}

func (s stubAdapter) DetectRoot(scopeRoot string) string {
	if s.detectDir == "" {
		return filepath.Join(scopeRoot, "missing-"+s.id)
	}
	return filepath.Join(scopeRoot, s.detectDir)
}

func (s stubAdapter) Install(_ Context, _ WriterFunc) (InstallResult, error) {
	return InstallResult{}, errors.New("not implemented")
}

func (s stubAdapter) Plan(_ Context) []PlannedArtifact {
	return s.plan
}

func (s stubAdapter) Verify(_ Context) error {
	return nil
}

func writeFileWriter(path string, data []byte, perm os.FileMode) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	old, err := os.ReadFile(path)
	if err == nil && string(old) == string(data) {
		return false, nil
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return false, err
	}
	return true, nil
}

func TestSupportedToolsSorted(t *testing.T) {
	adapters := map[string]Adapter{
		"zeta":  stubAdapter{id: "zeta"},
		"alpha": stubAdapter{id: "alpha"},
		"beta":  stubAdapter{id: "beta"},
	}
	got := SupportedTools(adapters)
	want := []string{"alpha", "beta", "zeta"}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected tool order: got=%v want=%v", got, want)
	}
}

func TestValidateSelectedTools(t *testing.T) {
	adapters := map[string]Adapter{"alpha": stubAdapter{id: "alpha"}}
	if err := ValidateSelectedTools([]string{"alpha"}, adapters); err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if ValidateSelectedTools([]string{"beta"}, adapters) == nil {
		t.Fatal("expected error for unsupported tool")
	}
}

func TestDetectTools(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "alpha-root"), 0o755); err != nil {
		t.Fatal(err)
	}
	adapters := map[string]Adapter{
		"alpha": stubAdapter{id: "alpha", detectDir: "alpha-root"},
		"beta":  stubAdapter{id: "beta"},
	}
	detected := DetectTools(root, adapters)
	if len(detected) != 1 || detected[0] != "alpha" {
		t.Fatalf("unexpected detect list: %v", detected)
	}
}

func TestDetectToolsIgnoresNonDirectoryCollisions(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "alpha-root")
	if err := os.WriteFile(filePath, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "beta-root"), 0o755); err != nil {
		t.Fatal(err)
	}

	adapters := map[string]Adapter{
		"alpha": stubAdapter{id: "alpha", detectDir: "alpha-root"},
		"beta":  stubAdapter{id: "beta", detectDir: "beta-root"},
	}
	detected := DetectTools(root, adapters)
	if len(detected) != 1 || detected[0] != "beta" {
		t.Fatalf("unexpected detect list: %v", detected)
	}
}

func TestInstallPlannedArtifactsCounts(t *testing.T) {
	tmp := t.TempDir()
	plan := []PlannedArtifact{
		{Kind: ArtifactHook, Path: filepath.Join(tmp, "hook.sh"), Content: "echo hook", Perm: 0o750},
		{Kind: ArtifactSettings, Path: filepath.Join(tmp, "settings.conf"), Content: "ok", Perm: 0o644},
	}
	res, err := InstallPlannedArtifacts(plan, writeFileWriter)
	if err != nil {
		t.Fatalf("install error: %v", err)
	}
	if res.Applied != 2 || res.Noop != 0 {
		t.Fatalf("unexpected result %+v", res)
	}
}

func TestResolveHomeScopedPath(t *testing.T) {
	if got := ResolveHomeScopedPath("", "rel"); got != "rel" {
		t.Fatalf("expected rel when home empty, got %s", got)
	}
	home := filepath.Join(t.TempDir(), "home")
	got := ResolveHomeScopedPath(home, "bin")
	rel, err := filepath.Rel(home, got)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("expected joined path under home, got %s (home=%s)", got, home)
	}
}

func TestDefaultAdaptersContainsExpectedTools(t *testing.T) {
	adapters := DefaultAdapters()
	for _, id := range []string{"claude", "codex", "cursor", "github-copilot", "opencode"} {
		if _, ok := adapters[id]; !ok {
			t.Fatalf("expected adapter %q", id)
		}
	}
}

func TestNoopAdapterLifecycle(t *testing.T) {
	a := NewNoopAdapter("cursor", ".cursor")
	if a.ID() != "cursor" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot("/repo"), ".cursor") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot("/repo"))
	}
	res, err := a.Install(Context{}, writeFileWriter)
	if err != nil || res.Noop != 1 {
		t.Fatalf(errUnexpectedInstFmt, res, err)
	}
	if a.Plan(Context{}) != nil {
		t.Fatal("expected nil plan")
	}
	if err := a.Verify(Context{}); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
	res, err = a.Uninstall(Context{})
	if err != nil || res.Noop != 1 {
		t.Fatalf(errUnexpectedUninstFmt, res, err)
	}
}

func TestGenericAdapterPlanInstallVerify(t *testing.T) {
	tmp := t.TempDir()
	a := NewGenericAdapter("alpha", ".alpha")
	ctx := Context{ScopeRoot: tmp, HomeDir: tmp}
	if a.ID() != "alpha" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(tmp), ".alpha") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(tmp))
	}
	if got := len(a.Plan(ctx)); got != 3 {
		t.Fatalf("plan len=%d want 3", got)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf(errInstallFmt, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
}

func TestOpenCodeAdapterPlanVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	a := NewOpenCodeAdapter()
	ctx := Context{ScopeRoot: tmp, HomeDir: filepath.Join(tmp, "home")}
	if a.ID() != "opencode" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(tmp), ".opencode") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(tmp))
	}
	if got := len(a.Plan(ctx)); got != 1 {
		t.Fatalf("plan len=%d want 1", got)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf(errInstallFmt, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
	res, err := a.Uninstall(ctx)
	if err != nil || res.Applied != 1 {
		t.Fatalf(errUnexpectedUninstFmt, res, err)
	}
	if _, err := a.Uninstall(ctx); err != nil {
		t.Fatalf("second uninstall error: %v", err)
	}
}

func TestCodexAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	a := NewCodexAdapter()
	ctx := Context{ScopeRoot: filepath.Join(tmp, "repo"), HomeDir: home}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if a.ID() != "codex" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".codex") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf(errInstallFmt, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
	res, err := a.Uninstall(ctx)
	if err != nil || res.Applied != 1 {
		t.Fatalf(errUnexpectedUninstFmt, res, err)
	}
}

func TestClaudeAdapterPlanInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: filepath.Join(tmp, "repo"), HomeDir: home}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewClaudeAdapter()
	if a.ID() != "claude" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".claude") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	if got := len(a.Plan(ctx)); got != 3 {
		t.Fatalf("plan len=%d want 3", got)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf(errInstallFmt, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
	res, err := a.Uninstall(ctx)
	if err != nil {
		t.Fatalf("uninstall error: %v", err)
	}
	if res.Applied == 0 {
		t.Fatalf("expected uninstall to remove artifacts, got %+v", res)
	}
}

func TestCodexManagedBlockHelpers(t *testing.T) {
	base := "hello\n"
	updated, err := upsertManagedInstructionBlock(filepath.Join(t.TempDir(), "missing", agentsFileName))
	if err != nil {
		t.Fatalf("upsert missing: %v", err)
	}
	if !strings.Contains(updated, ccpManagedBlockStart) {
		t.Fatalf("missing block in %q", updated)
	}
	if !strings.Contains(updated, "Use `ccp` as the command prefix for every executable in shell commands, including chained (`&&`, `||`) and piped (`|`) expressions.") {
		t.Fatalf("missing ccp prefix guidance in %q", updated)
	}
	if !strings.Contains(updated, "`ccp echo chain-ok && ccp echo chain-done`") {
		t.Fatalf("missing chaining example in %q", updated)
	}
	if got := normalizeManagedFile(base); got != "hello\n" {
		t.Fatalf("unexpected normalized output %q", got)
	}

	tmp := t.TempDir()
	p := filepath.Join(tmp, agentsFileName)
	if err := os.WriteFile(p, []byte("start\n"+ccpManagedBlockTemplate()+"tail\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, changed, removeAll, err := removeManagedInstructionBlock(p)
	if err != nil || !changed || removeAll {
		t.Fatalf("unexpected remove result changed=%v removeAll=%v err=%v", changed, removeAll, err)
	}
	if strings.Contains(out, ccpManagedBlockStart) {
		t.Fatalf("expected block removed, got %q", out)
	}
}

func TestClaudeHookRemovalHelpers(t *testing.T) {
	tmp := t.TempDir()
	settings := filepath.Join(tmp, "settings.json")
	hook := filepath.Join(tmp, "ccp-rewrite.sh")
	if err := os.WriteFile(settings, []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"`+strings.ReplaceAll(hook, "\\", "\\\\")+`"}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := removeClaudePreToolUseHook(settings, hook)
	if err != nil || !changed {
		t.Fatalf("expected changed=true err=nil, got changed=%v err=%v", changed, err)
	}
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		t.Fatalf("expected settings removed when empty, err=%v", err)
	}
	if _, err := removeFileIfExists(filepath.Join(tmp, "missing")); err != nil {
		t.Fatalf("removeFileIfExists missing: %v", err)
	}
}

func TestClaudeHookScriptPrefixesEachChainedSegment(t *testing.T) {
	script := claudeHookScriptContent()
	if !strings.Contains(script, `grep -Eq "['\"\\\\]|\\$\\(|\\$\\{|<<"`) {
		t.Fatalf("expected conservative complexity fallback guard in hook script, got: %s", script)
	}
	if !strings.Contains(script, `gsub("(^|\\|\\||&&|\\||;)\\s*(?!ccp\\b)"; "\\1 ccp ")`) {
		t.Fatalf("expected chained-segment rewrite rule in hook script, got: %s", script)
	}
	if !strings.Contains(script, `if [ "$REWRITTEN_CMD" = "$CMD" ]; then`) {
		t.Fatalf("expected no-op guard when rewrite does not change command, got: %s", script)
	}
	if !strings.Contains(script, `if ! sh -n -c "$REWRITTEN_CMD" >/dev/null 2>&1; then`) {
		t.Fatalf("expected shell syntax verification guard for rewritten command, got: %s", script)
	}
	if !strings.Contains(script, `--arg cmd "$REWRITTEN_CMD"`) {
		t.Fatalf("expected updated input payload to use rewritten command, got: %s", script)
	}
}

func TestCodexPlanAndUpsertBranches(t *testing.T) {
	a := NewCodexAdapter()
	ctx := Context{HomeDir: filepath.Join(t.TempDir(), "home")}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".codex", "AGENTS.md")) {
		t.Fatalf("unexpected plan: %+v", plan)
	}

	tmp := t.TempDir()
	p := filepath.Join(tmp, agentsFileName)
	if err := os.WriteFile(p, []byte("prefix\nsuffix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := upsertManagedInstructionBlock(p)
	if err != nil {
		t.Fatalf("upsert append branch: %v", err)
	}
	if !strings.Contains(out, ccpManagedBlockStart) || !strings.Contains(out, "prefix") {
		t.Fatalf("unexpected appended content: %q", out)
	}

	withBlock := "before\n" + ccpManagedBlockTemplate() + "\nafter\n"
	if err := os.WriteFile(p, []byte(withBlock), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = upsertManagedInstructionBlock(p)
	if err != nil {
		t.Fatalf("upsert replace branch: %v", err)
	}
	if strings.Count(out, ccpManagedBlockStart) != 1 {
		t.Fatalf("expected single managed block, got %q", out)
	}
}

func TestCodexUpsertOnMissingFileUsesCanonicalTemplate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "missing", agentsFileName)
	out, err := upsertManagedInstructionBlock(p)
	if err != nil {
		t.Fatalf("upsert missing file: %v", err)
	}
	if out != ccpManagedBlockTemplate() {
		t.Fatalf("expected canonical template for missing file, got %q", out)
	}
}

func TestGitHubCopilotAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: filepath.Join(tmp, "repo"), HomeDir: home}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewGitHubCopilotAdapter()
	if a.ID() != "github-copilot" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".github") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	if got := len(a.Plan(ctx)); got != 1 {
		t.Fatalf("plan len=%d want 1", got)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf(errInstallFmt, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
	res, err := a.Uninstall(ctx)
	if err != nil || res.Applied != 1 {
		t.Fatalf(errUnexpectedUninstFmt, res, err)
	}
}

func TestOpenCodeVerifyErrorBranchesAndGlobalRoot(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	scope := home
	ctx := Context{ScopeRoot: scope, HomeDir: home}
	if !strings.Contains(opencodeConfigRoot(ctx), filepath.Join(".config", "opencode")) {
		t.Fatalf("expected global opencode root, got %q", opencodeConfigRoot(ctx))
	}

	a := NewOpenCodeAdapter()
	pluginPath := filepath.Join(opencodeConfigRoot(ctx), "plugins", opencodePluginName)
	if err := os.MkdirAll(filepath.Dir(pluginPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pluginPath, []byte("export default {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if a.Verify(ctx) == nil {
		t.Fatal("expected verify failure for invalid plugin content")
	}
}

func TestOpenCodePluginScriptUsesSafeChainRewrite(t *testing.T) {
	script := opencodePluginContent()
	if !strings.Contains(script, `if (/['"\\]|\$\(|\$\{|<</.test(command))`) {
		t.Fatalf("expected conservative complexity fallback guard in OpenCode plugin, got: %s", script)
	}
	if !strings.Contains(script, `command.replace(/(^|\|\||&&|\||;)\s*(?!ccp\b)/g, "$1 ccp ")`) {
		t.Fatalf("expected chained-segment rewrite rule in OpenCode plugin, got: %s", script)
	}
	if !strings.Contains(script, `if (rewritten === command)`) {
		t.Fatalf("expected no-op guard when rewrite does not change command, got: %s", script)
	}
	if !strings.Contains(script, `output.args.command = rewritten;`) {
		t.Fatalf("expected OpenCode plugin to persist rewritten command, got: %s", script)
	}
}

func TestClaudeHookRemovalNoChangeBranches(t *testing.T) {
	tmp := t.TempDir()
	settings := filepath.Join(tmp, "settings.json")
	hook := filepath.Join(tmp, "ccp-rewrite.sh")

	if changed, err := removeClaudePreToolUseHook(settings, hook); err != nil || changed {
		t.Fatalf("expected no change when settings missing, changed=%v err=%v", changed, err)
	}
	if err := os.WriteFile(settings, []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := removeClaudePreToolUseHook(settings, hook)
	if err != nil || !changed {
		t.Fatalf("expected removal on invalid json, changed=%v err=%v", changed, err)
	}
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		t.Fatalf("expected settings removed for invalid json, err=%v", err)
	}
}
