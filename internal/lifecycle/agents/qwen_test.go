package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
