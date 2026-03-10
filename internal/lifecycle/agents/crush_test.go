package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestCrushContextPathsFallback(t *testing.T) {
	if got := crushContextPaths("unexpected"); len(got) != 0 {
		t.Fatalf("expected no crush context paths for non-slice input, got=%v", got)
	}
}
