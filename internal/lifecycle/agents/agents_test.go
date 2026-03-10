package agents

import (
	"encoding/json"
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
	errUnexpectedUninstFmt = "unexpected uninstall result %+v err=%v"
	errVerifyFmt           = "verify error: %v"
	agentsFileName         = "AGENTS.md"
	iflowFileName          = "IFLOW.md"
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

func TestResolveRepoScopedPath(t *testing.T) {
	if got := ResolveRepoScopedPath("", "rel"); got != "rel" {
		t.Fatalf("expected rel when scope root empty, got %s", got)
	}
	root := filepath.Join(t.TempDir(), "repo")
	got := ResolveRepoScopedPath(root, "rules/ccp.md")
	rel, err := filepath.Rel(root, got)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("expected joined path under repo root, got %s (root=%s)", got, root)
	}
}

func TestDefaultAdaptersContainsExpectedTools(t *testing.T) {
	adapters := DefaultAdapters()
	for _, id := range []string{"aider", "auggie", "antigravity", "amazon-q", "codebuddy", "cline", "claude", "codex", "continue", "crush", "cursor", "factory", "gemini", "github-copilot", "iflow", "kiro", "kilocode", "opencode", "pi", "qoder", "qwen", "roocode", "trae", "windsurf"} {
		if _, ok := adapters[id]; !ok {
			t.Fatalf("expected adapter %q", id)
		}
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

func TestManagedHomeRuleFileAdapterSeparatesDetectionFromInstallTarget(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	repo := filepath.Join(tmp, "repo")
	ctx := Context{ScopeRoot: repo, HomeDir: home}
	a := NewManagedHomeRuleFileAdapter(
		"alpha",
		".alpha",
		filepath.Join(".alpha", "rules", "ccp.md"),
		"missing alpha rule file: %s",
		"missing alpha managed guidance in %s",
		func() string { return "ccp-managed\n" },
		[]string{"ccp-managed"},
	)

	if got := a.DetectRoot(repo); got != filepath.Join(repo, ".alpha") {
		t.Fatalf("unexpected detect root %q", got)
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 {
		t.Fatalf("plan len=%d want 1", len(plan))
	}
	if !strings.HasPrefix(plan[0].Path, home) {
		t.Fatalf("expected home-scoped install target, got %s", plan[0].Path)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf("install error: %v", err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf("verify error: %v", err)
	}
}

func TestManagedInstructionFileAdapterUninstallPreservesUserContent(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: filepath.Join(tmp, "repo"), HomeDir: home}
	a := NewManagedInstructionFileAdapter(
		"alpha",
		".alpha",
		filepath.Join(".alpha", "AGENTS.md"),
		"missing alpha agents file: %s",
		"missing alpha managed markers in %s",
	)
	target := filepath.Join(home, ".alpha", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("user header\n\n"+ccpManagedBlockTemplate()), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := a.Uninstall(ctx)
	if err != nil || res.Applied != 1 {
		t.Fatalf(errUnexpectedUninstFmt, res, err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target after uninstall: %v", err)
	}
	if string(got) != "user header\n" {
		t.Fatalf("unexpected content after uninstall: %q", string(got))
	}
}

func TestManagedRepoRuleFileAdapterUninstallPreservesSiblingFiles(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	ctx := Context{ScopeRoot: repo, HomeDir: filepath.Join(tmp, "home")}
	a := NewManagedRepoRuleFileAdapter(
		"alpha",
		".alpha",
		filepath.Join(".alpha", "rules", "ccp.md"),
		"missing alpha rule file: %s",
		"missing alpha managed guidance in %s",
		func() string { return "ccp-managed\n" },
		[]string{"ccp-managed"},
	)
	target := filepath.Join(repo, ".alpha", "rules", "ccp.md")
	sibling := filepath.Join(repo, ".alpha", "rules", "user.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("ccp-managed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sibling, []byte("keep-me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := a.Uninstall(ctx)
	if err != nil || res.Applied != 1 {
		t.Fatalf(errUnexpectedUninstFmt, res, err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected target removed, err=%v", err)
	}
	if got, err := os.ReadFile(sibling); err != nil || string(got) != "keep-me\n" {
		t.Fatalf("expected sibling preserved, got=%q err=%v", string(got), err)
	}
}

func TestCostrictAliasReusesRooCodeCanonicalTarget(t *testing.T) {
	tmp := t.TempDir()
	ctx := Context{ScopeRoot: filepath.Join(tmp, "repo"), HomeDir: filepath.Join(tmp, "home")}
	adapters := DefaultAdapters()

	aliasPlan := adapters[NormalizeToolID(string(AgentCostrict))].Plan(ctx)
	canonicalPlan := adapters[string(AgentRooCode)].Plan(ctx)
	if len(aliasPlan) != 1 || len(canonicalPlan) != 1 {
		t.Fatalf("unexpected alias/canonical plan lengths: alias=%d canonical=%d", len(aliasPlan), len(canonicalPlan))
	}
	if aliasPlan[0].Path != canonicalPlan[0].Path {
		t.Fatalf("expected alias target %s to match canonical target %s", aliasPlan[0].Path, canonicalPlan[0].Path)
	}
}

func TestQwenAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".qwen"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewQwenAdapter()
	if a.ID() != "qwen" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".qwen") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 2 || !strings.HasSuffix(plan[0].Path, filepath.Join(".qwen", "settings.json")) || !strings.HasSuffix(plan[1].Path, filepath.Join(".qwen", "AGENTS.md")) {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf(errInstallFmt, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
	res, err := a.Uninstall(ctx)
	if err != nil || res.Applied != 2 {
		t.Fatalf(errUnexpectedUninstFmt, res, err)
	}
}

func TestQoderAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".qoder"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewQoderAdapter()
	if a.ID() != "qoder" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".qoder") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".qoder", "AGENTS.md")) {
		t.Fatalf("unexpected plan: %+v", plan)
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

func TestCrushAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".crush"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewCrushAdapter()
	if a.ID() != "crush" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".crush") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".config", "crush", "CRUSH.md")) || !strings.HasPrefix(plan[0].Path, home) {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf(errInstallFmt, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
	res, err := a.Uninstall(ctx)
	if err != nil || res.Applied != 2 {
		t.Fatalf(errUnexpectedUninstFmt, res, err)
	}
}

func TestFactoryAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".factory"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewFactoryAdapter()
	if a.ID() != "factory" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".factory") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".factory", "AGENTS.md")) {
		t.Fatalf("unexpected plan: %+v", plan)
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

func TestAuggieAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: filepath.Join(tmp, "home")}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".augment"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewAuggieAdapter()
	if a.ID() != "auggie" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".augment") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, agentsFileName) {
		t.Fatalf("unexpected plan: %+v", plan)
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

func TestCodeBuddyAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".codebuddy"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewCodeBuddyAdapter()
	if a.ID() != "codebuddy" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".codebuddy") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 2 || !strings.HasSuffix(plan[0].Path, filepath.Join(".codebuddy", "hooks", codebuddyHookScriptName)) || !strings.HasSuffix(plan[1].Path, filepath.Join(".codebuddy", codebuddySettingsName)) {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf(errInstallFmt, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
	res, err := a.Uninstall(ctx)
	if err != nil || res.Applied != 2 {
		t.Fatalf(errUnexpectedUninstFmt, res, err)
	}
}

func TestCodeBuddySettingsUseHook(t *testing.T) {
	tmp := t.TempDir()
	settings := filepath.Join(tmp, codebuddySettingsName)
	hook := filepath.Join(tmp, codebuddyHookScriptName)
	escapedHook := strings.ReplaceAll(hook, "\\", "\\\\")
	content := "{\n  \"hooks\": {\n    \"PreToolUse\": [\n      {\n        \"matcher\": \"Bash\",\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"" + escapedHook + "\"\n          }\n        ]\n      }\n    ]\n  }\n}\n"
	if err := os.WriteFile(settings, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err := codebuddySettingsUseHook(settings, hook)
	if err != nil || !ok {
		t.Fatalf("expected settings to contain managed hook, ok=%v err=%v", ok, err)
	}
}

func TestCrushConfigHelpersUpsertAndVerify(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "crush.json")
	contextPath := filepath.Join(tmp, "CRUSH.md")

	if err := os.WriteFile(configPath, []byte("{\n  \"theme\": \"dark\",\n  \"options\": {\n    \"context_paths\": [\n      \"/tmp/team.md\"\n    ]\n  }\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	updated, err := upsertCrushConfig(configPath, contextPath)
	if err != nil {
		t.Fatalf("upsertCrushConfig: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(updated), &root); err != nil {
		t.Fatalf("unmarshal updated crush config: %v", err)
	}
	if root["theme"] != "dark" {
		t.Fatalf("expected preserved theme, got: %#v", root)
	}
	options, _ := root["options"].(map[string]any)
	if options == nil || !slicesContainsPath(crushContextPaths(options["context_paths"]), contextPath) {
		t.Fatalf("expected preserved theme and managed context path, got: %s", updated)
	}

	if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err := crushConfigUsesContext(configPath, contextPath)
	if err != nil || !ok {
		t.Fatalf("expected crush config to use context, ok=%v err=%v", ok, err)
	}
}

func TestCrushConfigHelpersRemoveBranches(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "crush.json")
	contextPath := filepath.Join(tmp, "CRUSH.md")

	if err := os.WriteFile(configPath, []byte("{\n  \"theme\": \"dark\",\n  \"options\": {\n    \"context_paths\": [\n      \""+strings.ReplaceAll(contextPath, "\\", "\\\\")+"\"\n    ]\n  }\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, changed, removeAll, err := removeCrushContextPath(configPath, contextPath)
	if err != nil || !changed || removeAll {
		t.Fatalf("unexpected removeCrushContextPath result changed=%v removeAll=%v err=%v", changed, removeAll, err)
	}
	if strings.Contains(removed, contextPath) || !strings.Contains(removed, `"theme": "dark"`) {
		t.Fatalf("expected managed context removed and unrelated config preserved, got: %s", removed)
	}

	if err := os.WriteFile(configPath, []byte("{\n  \"options\": {\n    \"context_paths\": [\n      \""+strings.ReplaceAll(contextPath, "\\", "\\\\")+"\"\n    ]\n  }\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, changed, removeAll, err = removeCrushContextPath(configPath, contextPath)
	if err != nil || !changed || !removeAll {
		t.Fatalf("expected remove-all branch, changed=%v removeAll=%v err=%v", changed, removeAll, err)
	}
}

func TestQwenSettingsHelpersReadUpsertAndVerify(t *testing.T) {
	tmp := t.TempDir()
	settingsPath := filepath.Join(tmp, "settings.json")

	if err := os.WriteFile(settingsPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := readQwenSettings(settingsPath)
	if err != nil || len(root) != 0 {
		t.Fatalf("expected empty qwen settings map, root=%v err=%v", root, err)
	}

	if err := os.WriteFile(settingsPath, []byte("{\n  \"theme\": \"light\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	updated, err := upsertQwenSettings(settingsPath)
	if err != nil {
		t.Fatalf("upsertQwenSettings: %v", err)
	}
	if !strings.Contains(updated, `"theme": "light"`) || !strings.Contains(updated, `"fileName": "AGENTS.md"`) {
		t.Fatalf("expected qwen settings update to preserve unrelated config, got: %s", updated)
	}

	if err := os.WriteFile(settingsPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err := qwenSettingsUseAgents(settingsPath)
	if err != nil || !ok {
		t.Fatalf("expected qwen settings to use AGENTS.md, ok=%v err=%v", ok, err)
	}
}

func TestQwenSettingsHelpersRemoveBranches(t *testing.T) {
	tmp := t.TempDir()
	settingsPath := filepath.Join(tmp, "settings.json")

	if err := os.WriteFile(settingsPath, []byte("{\n  \"theme\": \"light\",\n  \"context\": {\n    \"fileName\": \"AGENTS.md\"\n  }\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, changed, removeAll, err := removeQwenSettings(settingsPath)
	if err != nil || !changed || removeAll {
		t.Fatalf("unexpected removeQwenSettings result changed=%v removeAll=%v err=%v", changed, removeAll, err)
	}
	if strings.Contains(removed, `"fileName": "AGENTS.md"`) || !strings.Contains(removed, `"theme": "light"`) {
		t.Fatalf("expected AGENTS.md removed and theme preserved, got: %s", removed)
	}

	if err := os.WriteFile(settingsPath, []byte("{\n  \"context\": {\n    \"fileName\": \"OTHER.md\"\n  }\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, changed, removeAll, err = removeQwenSettings(settingsPath)
	if err != nil || changed || removeAll {
		t.Fatalf("expected no-op removeQwenSettings for non-AGENTS value, changed=%v removeAll=%v err=%v", changed, removeAll, err)
	}
}

func TestWindsurfHooksConfigHelpersUpsertAndVerify(t *testing.T) {
	tmp := t.TempDir()
	hooksPath := filepath.Join(tmp, "hooks.json")
	managedHook := filepath.Join(tmp, "ccp-block.sh")

	updated, err := upsertWindsurfHooksConfig(hooksPath, managedHook)
	if err != nil {
		t.Fatalf("upsertWindsurfHooksConfig: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(updated), &root); err != nil {
		t.Fatalf("unmarshal updated windsurf hooks: %v", err)
	}
	if !windsurfHookEntriesContain(normalizeWindsurfHookEntries(root["pre_run_command"]), managedHook) {
		t.Fatalf("expected managed hook path in config, got: %s", updated)
	}
	if err := os.WriteFile(hooksPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err := windsurfHooksConfigHasEntry(hooksPath, managedHook)
	if err != nil || !ok {
		t.Fatalf("expected windsurf config to contain managed hook, ok=%v err=%v", ok, err)
	}
}

func TestWindsurfHooksConfigHelpersRemoveBranches(t *testing.T) {
	tmp := t.TempDir()
	hooksPath := filepath.Join(tmp, "hooks.json")
	managedHook := filepath.Join(tmp, "ccp-block.sh")
	otherHook := filepath.Join(tmp, "other.sh")

	content := "{\n  \"pre_run_command\": [\n    {\n      \"name\": \"ccp-pre-run-command\",\n      \"command\": \"" + strings.ReplaceAll(managedHook, "\\", "\\\\") + "\",\n      \"enabled\": true\n    },\n    {\n      \"name\": \"other\",\n      \"command\": \"" + strings.ReplaceAll(otherHook, "\\", "\\\\") + "\",\n      \"enabled\": true\n    }\n  ]\n}\n"
	if err := os.WriteFile(hooksPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, changed, removeAll, err := removeWindsurfHooksConfig(hooksPath, managedHook)
	if err != nil || !changed || removeAll {
		t.Fatalf("unexpected removeWindsurfHooksConfig result changed=%v removeAll=%v err=%v", changed, removeAll, err)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(removed), &root); err != nil {
		t.Fatalf("unmarshal removed windsurf hooks: %v", err)
	}
	entries := normalizeWindsurfHookEntries(root["pre_run_command"])
	if windsurfHookEntriesContain(entries, managedHook) || !windsurfHookEntriesContain(entries, otherHook) {
		t.Fatalf("expected managed hook removed and other hook preserved, got: %s", removed)
	}

	if err := os.WriteFile(hooksPath, []byte("{\n  \"pre_run_command\": [\n    {\n      \"name\": \"ccp-pre-run-command\",\n      \"command\": \""+strings.ReplaceAll(managedHook, "\\", "\\\\")+"\",\n      \"enabled\": true\n    }\n  ]\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, changed, removeAll, err = removeWindsurfHooksConfig(hooksPath, managedHook)
	if err != nil || !changed || !removeAll {
		t.Fatalf("expected remove-all windsurf branch, changed=%v removeAll=%v err=%v", changed, removeAll, err)
	}
}

func TestSmallHelperFallbacks(t *testing.T) {
	if got := crushContextPaths("unexpected"); len(got) != 0 {
		t.Fatalf("expected no crush context paths for non-slice input, got=%v", got)
	}
	if got := normalizeWindsurfHookEntries("unexpected"); got != nil {
		t.Fatalf("expected nil windsurf entries for non-slice input, got=%v", got)
	}
}

func TestNoopAdapter(t *testing.T) {
	a := NewNoopAdapter("noop", ".noop")
	ctx := Context{ScopeRoot: t.TempDir(), HomeDir: t.TempDir()}
	if a.ID() != "noop" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".noop") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	if plan := a.Plan(ctx); len(plan) != 0 {
		t.Fatalf("expected empty noop plan, got %+v", plan)
	}
	res, err := a.Install(ctx, writeFileWriter)
	if err != nil || res.Noop != 1 {
		t.Fatalf("unexpected noop install result %+v err=%v", res, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf("unexpected noop verify error: %v", err)
	}
	res, err = a.Uninstall(ctx)
	if err != nil || res.Noop != 1 {
		t.Fatalf("unexpected noop uninstall result %+v err=%v", res, err)
	}
}

func TestIFlowAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".iflow"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewIFlowAdapter()
	if a.ID() != "iflow" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".iflow") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".iflow", iflowFileName)) {
		t.Fatalf("unexpected plan: %+v", plan)
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

func TestPiAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: filepath.Join(tmp, "home")}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".pi"), 0o755); err != nil {
		t.Fatal(err)
	}

	a := NewPiAdapter()
	if a.ID() != "pi" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".pi") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, agentsFileName) {
		t.Fatalf("unexpected plan: %+v", plan)
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

func TestAiderConfigHelpersPreserveOtherReadEntries(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, aiderConfigPath)
	readPath := filepath.Join(tmp, aiderRulesPath)
	if err := os.WriteFile(configPath, []byte("read:\n  - CONVENTIONS.md\nmodel: sonnet\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	updated, err := upsertAiderReadConfig(configPath, readPath)
	if err != nil {
		t.Fatalf("upsert aider config: %v", err)
	}
	if !strings.Contains(updated, "- CONVENTIONS.md") || !strings.Contains(updated, "- "+readPath) {
		t.Fatalf("expected both read entries preserved, got: %s", updated)
	}
	if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
		t.Fatalf("write updated aider config: %v", err)
	}

	updated, changed, removeAll, err := removeAiderReadConfig(configPath, readPath)
	if err != nil || !changed || removeAll {
		t.Fatalf("unexpected remove result changed=%v removeAll=%v err=%v", changed, removeAll, err)
	}
	if strings.Contains(updated, readPath) {
		t.Fatalf("expected aider read path removed, got: %s", updated)
	}
	if !strings.Contains(updated, "CONVENTIONS.md") || !strings.Contains(updated, "model: sonnet") {
		t.Fatalf("expected unrelated config preserved, got: %s", updated)
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

func TestAiderAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(scopeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewAiderAdapter()
	if a.ID() != "aider" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".aider.conf.yml") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 2 || !strings.HasSuffix(plan[0].Path, ".aider.conf.yml") || !strings.HasSuffix(plan[1].Path, ".aider.rules.md") {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf(errInstallFmt, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
	res, err := a.Uninstall(ctx)
	if err != nil || res.Applied != 2 {
		t.Fatalf(errUnexpectedUninstFmt, res, err)
	}
}

func TestGeminiAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: filepath.Join(tmp, "repo"), HomeDir: home}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewGeminiAdapter()
	if a.ID() != "gemini" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".gemini") {
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

func TestCursorAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: filepath.Join(tmp, "home")}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewCursorAdapter()
	if a.ID() != "cursor" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".cursor") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".cursor", "rules", "ccp.mdc")) {
		t.Fatalf("unexpected plan: %+v", plan)
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

func TestAmazonQAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: filepath.Join(tmp, "home")}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".amazonq"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewAmazonQAdapter()
	if a.ID() != "amazon-q" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".amazonq") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".amazonq", "rules", "ccp.md")) {
		t.Fatalf("unexpected plan: %+v", plan)
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

func TestAntigravityAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewAntigravityAdapter()
	if a.ID() != "antigravity" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".agent") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".gemini", "GEMINI.md")) || !strings.HasPrefix(plan[0].Path, home) {
		t.Fatalf("unexpected plan: %+v", plan)
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

func TestAntigravityPlanReusesGeminiFamilyTarget(t *testing.T) {
	tmp := t.TempDir()
	ctx := Context{ScopeRoot: filepath.Join(tmp, "repo"), HomeDir: filepath.Join(tmp, "home")}
	antigravityPlan := NewAntigravityAdapter().Plan(ctx)
	geminiPlan := NewGeminiAdapter().Plan(ctx)
	if len(antigravityPlan) != 1 || len(geminiPlan) != 1 {
		t.Fatalf("unexpected antigravity/gemini plan lengths: antigravity=%d gemini=%d", len(antigravityPlan), len(geminiPlan))
	}
	if antigravityPlan[0].Path != geminiPlan[0].Path {
		t.Fatalf("expected antigravity target %s to match gemini target %s", antigravityPlan[0].Path, geminiPlan[0].Path)
	}
}

func TestKiroAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".kiro"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewKiroAdapter()
	if a.ID() != "kiro" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".kiro") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".kiro", "steering", "AGENTS.md")) {
		t.Fatalf("unexpected plan: %+v", plan)
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

func TestKilocodeAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".kilocode"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewKilocodeAdapter()
	if a.ID() != "kilocode" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".kilocode") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".config", "kilocode", "plugins", opencodePluginName)) || !strings.HasPrefix(plan[0].Path, home) {
		t.Fatalf("unexpected plan: %+v", plan)
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

func TestRooCodeAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".roo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewRooCodeAdapter()
	if a.ID() != "roocode" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".roo") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".roo", "rules", "ccp.md")) || !strings.HasPrefix(plan[0].Path, home) {
		t.Fatalf("unexpected plan: %+v", plan)
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

func TestCursorRuleContentUsesMinimalAlwaysApplyMetadata(t *testing.T) {
	content := cursorRuleContent()
	if !strings.HasPrefix(content, "---\n") {
		t.Fatalf("expected frontmatter start, got: %s", content)
	}
	if !strings.Contains(content, "description: Route shell commands through ccp") {
		t.Fatalf("expected description metadata, got: %s", content)
	}
	if !strings.Contains(content, "alwaysApply: true") {
		t.Fatalf("expected alwaysApply metadata, got: %s", content)
	}
	if !strings.Contains(content, "Use `ccp` as the command prefix for every executable in shell commands") {
		t.Fatalf("expected canonical ccp guidance, got: %s", content)
	}
	if strings.Contains(content, ccpManagedBlockStart) || strings.Contains(content, ccpManagedBlockEnd) {
		t.Fatalf("did not expect managed block markers in cursor rule, got: %s", content)
	}
}

func TestAmazonQRuleContentUsesCanonicalGuidanceWithoutCursorMetadata(t *testing.T) {
	content := amazonQRuleContent()
	if !strings.Contains(content, "## CCP Integration (Managed)") {
		t.Fatalf("expected managed heading, got: %s", content)
	}
	if !strings.Contains(content, "Use `ccp` as the command prefix for every executable in shell commands") {
		t.Fatalf("expected canonical ccp guidance, got: %s", content)
	}
	if !strings.Contains(content, ccpRawEscapeHatch) {
		t.Fatalf("expected raw escape hatch note, got: %s", content)
	}
	if strings.Contains(content, "alwaysApply: true") || strings.Contains(content, "description:") {
		t.Fatalf("did not expect cursor frontmatter in amazon q rule, got: %s", content)
	}
	if strings.Contains(content, ccpManagedBlockStart) || strings.Contains(content, ccpManagedBlockEnd) {
		t.Fatalf("did not expect managed block markers in amazon q rule, got: %s", content)
	}
}

func TestKiroSteeringContentUsesCanonicalGuidance(t *testing.T) {
	content := ccpManagedBlockTemplate()
	if !strings.Contains(content, ccpManagedBlockStart) || !strings.Contains(content, ccpManagedBlockEnd) {
		t.Fatalf("expected managed block markers, got: %s", content)
	}
	if !strings.Contains(content, "Use `ccp` as the command prefix for every executable in shell commands") {
		t.Fatalf("expected canonical ccp guidance, got: %s", content)
	}
}

func TestKilocodePluginContentUsesOpenCodeFamilyHook(t *testing.T) {
	content := opencodePluginContent()
	if !strings.Contains(content, `"tool.execute.before"`) {
		t.Fatalf("expected tool.execute.before hook, got: %s", content)
	}
	if !strings.Contains(content, `input.tool !== "bash"`) {
		t.Fatalf("expected bash-only guard, got: %s", content)
	}
	if !strings.Contains(content, `output.args.command = rewritten`) {
		t.Fatalf("expected command rewrite, got: %s", content)
	}
}

func TestRooCodeRuleContentUsesCanonicalGuidance(t *testing.T) {
	content := roocodeRuleContent()
	if !strings.Contains(content, "## CCP Integration (Managed)") {
		t.Fatalf("expected managed heading, got: %s", content)
	}
	if !strings.Contains(content, "Use `ccp` as the command prefix for every executable in shell commands") {
		t.Fatalf("expected canonical ccp guidance, got: %s", content)
	}
	if strings.Contains(content, ccpManagedBlockStart) || strings.Contains(content, ccpManagedBlockEnd) {
		t.Fatalf("did not expect managed block markers in roocode rule, got: %s", content)
	}
}

func TestWindsurfAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".windsurf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewWindsurfAdapter()
	if a.ID() != "windsurf" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".windsurf") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 2 || !strings.HasSuffix(plan[0].Path, filepath.Join(".codeium", "windsurf", "hooks", "ccp-block.sh")) || !strings.HasSuffix(plan[1].Path, filepath.Join(".codeium", "windsurf", "hooks.json")) {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf(errInstallFmt, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
	res, err := a.Uninstall(ctx)
	if err != nil || res.Applied != 2 {
		t.Fatalf(errUnexpectedUninstFmt, res, err)
	}
}

func TestWindsurfRuleContentUsesAlwaysOnMetadata(t *testing.T) {
	content := windsurfHookScriptContent()
	for _, needle := range []string{
		"generated by ccp init for windsurf",
		"pre_run_command hook",
		"Use ccp as the command prefix for shell commands",
		"exit 2",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("expected canonical windsurf hook content %q, got: %s", needle, content)
		}
	}
}

func TestClineAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".clinerules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewClineAdapter()
	if a.ID() != "cline" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".clinerules") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join("Cline", "Hooks", "PreToolUse")) {
		t.Fatalf("unexpected plan: %+v", plan)
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

func TestContinueAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	home := filepath.Join(tmp, "home")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: home}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".continue"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewContinueAdapter()
	if a.ID() != "continue" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".continue") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 2 || !strings.HasSuffix(plan[0].Path, filepath.Join(".continue", "hooks", "ccp-rewrite.sh")) || !strings.HasSuffix(plan[1].Path, filepath.Join(".continue", "settings.json")) {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if _, err := a.Install(ctx, writeFileWriter); err != nil {
		t.Fatalf(errInstallFmt, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf(errVerifyFmt, err)
	}
	res, err := a.Uninstall(ctx)
	if err != nil || res.Applied != 2 {
		t.Fatalf(errUnexpectedUninstFmt, res, err)
	}
}

func TestTraeAdapterInstallVerifyAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	scopeRoot := filepath.Join(tmp, "repo")
	ctx := Context{ScopeRoot: scopeRoot, HomeDir: filepath.Join(tmp, "home")}
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".trae"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewTraeAdapter()
	if a.ID() != "trae" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".trae") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	plan := a.Plan(ctx)
	if len(plan) != 1 || !strings.HasSuffix(plan[0].Path, filepath.Join(".trae", "rules", "ccp.md")) {
		t.Fatalf("unexpected plan: %+v", plan)
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

func TestClineRuleContentUsesCanonicalGuidance(t *testing.T) {
	content := clineHookScriptContent()
	for _, needle := range []string{
		"generated by ccp init for cline",
		"execute_command",
		`"Decision":"block"`,
		"Use ccp as the command prefix for shell commands",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("expected canonical cline hook content %q, got: %s", needle, content)
		}
	}
}

func TestResolveClineHooksDirPrefersExistingGlobalDirectory(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	preferred := filepath.Join(home, "Cline", "Hooks")
	if err := os.MkdirAll(preferred, 0o755); err != nil {
		t.Fatal(err)
	}
	got := resolveClineHooksDir(Context{HomeDir: home})
	if got != preferred {
		t.Fatalf("expected existing hooks dir %s, got %s", preferred, got)
	}
}

func TestResolveClineHooksDirUsesDocumentsOverride(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	docs := filepath.Join(tmp, "docs")
	t.Setenv("XDG_DOCUMENTS_DIR", docs)
	got := resolveClineHooksDir(Context{HomeDir: home})
	want := filepath.Join(docs, "Cline", "Hooks")
	if got != want {
		t.Fatalf("expected documents override %s, got %s", want, got)
	}
}

func TestContinueRuleContentUsesCanonicalGuidance(t *testing.T) {
	content := claudeHookScriptContent()
	for _, needle := range []string{
		"generated by ccp init for claude",
		"optionally rewrite Bash command",
		"ccp auto-rewrite",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("expected canonical continue hook content %q, got: %s", needle, content)
		}
	}
}

func TestTraeRuleContentUsesCanonicalGuidance(t *testing.T) {
	content := traeRuleContent()
	if !strings.Contains(content, "## CCP Integration (Managed)") {
		t.Fatalf("expected managed heading, got: %s", content)
	}
	if !strings.Contains(content, "Use `ccp` as the command prefix for every executable in shell commands") {
		t.Fatalf("expected canonical ccp guidance, got: %s", content)
	}
	if !strings.Contains(content, ccpRawEscapeHatch) {
		t.Fatalf("expected raw escape hatch note, got: %s", content)
	}
	if strings.Contains(content, "alwaysApply: true") || strings.Contains(content, "trigger: always_on") {
		t.Fatalf("did not expect cursor or windsurf metadata in trae rule, got: %s", content)
	}
	if strings.Contains(content, ccpManagedBlockStart) || strings.Contains(content, ccpManagedBlockEnd) {
		t.Fatalf("did not expect managed block markers in trae rule, got: %s", content)
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
