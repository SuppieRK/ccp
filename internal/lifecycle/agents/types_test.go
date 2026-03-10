package agents

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

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

func TestDefaultAdaptersContainsExpectedTools(t *testing.T) {
	adapters := DefaultAdapters()
	for _, id := range []string{"aider", "auggie", "antigravity", "amazon-q", "codebuddy", "cline", "claude", "codex", "continue", "crush", "cursor", "factory", "gemini", "github-copilot", "iflow", "kiro", "kilocode", "opencode", "pi", "qoder", "qwen", "roocode", "trae", "windsurf"} {
		if _, ok := adapters[id]; !ok {
			t.Fatalf("expected adapter %q", id)
		}
	}
}
