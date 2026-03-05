package projectfiles

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	gitignorePathName = ".gitignore"
	ensureEntryErrFmt = "ensure entry: %v"
)

func TestEnsureGitignoreEntryUpdatesContent(t *testing.T) {
	cases := []struct {
		name    string
		initial string
		want    string
	}{
		{name: "appends", initial: "node_modules\n", want: "node_modules\n.ccp\n"},
		{name: "no-duplicate", initial: ".ccp\n", want: ".ccp\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, gitignorePathName)
			if err := os.WriteFile(path, []byte(tc.initial), 0o644); err != nil {
				t.Fatalf("write .gitignore: %v", err)
			}
			if err := EnsureGitignoreEntry(root, ".ccp"); err != nil {
				t.Fatalf(ensureEntryErrFmt, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read .gitignore: %v", err)
			}
			if got := string(b); got != tc.want {
				t.Fatalf("unexpected content: %q", got)
			}
		})
	}
}

func TestEnsureGitignoreEntryMissingNoop(t *testing.T) {
	root := t.TempDir()
	if err := EnsureGitignoreEntry(root, ".ccp"); err != nil {
		t.Fatalf(ensureEntryErrFmt, err)
	}
	if _, err := os.Stat(filepath.Join(root, gitignorePathName)); !os.IsNotExist(err) {
		t.Fatalf("expected .gitignore to remain absent, err=%v", err)
	}
}

func TestEnsureGitignoreEntryAppendsWhenMissingTrailingNewline(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, gitignorePathName)
	if err := os.WriteFile(path, []byte("node_modules"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	if err := EnsureGitignoreEntry(root, ".ccp"); err != nil {
		t.Fatalf(ensureEntryErrFmt, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if got := string(b); got != "node_modules\n.ccp\n" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestEnsureGitignoreEntryEmptyInputsNoop(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, gitignorePathName)
	if err := os.WriteFile(path, []byte("node_modules\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	if err := EnsureGitignoreEntry("   ", ".ccp"); err != nil {
		t.Fatalf(ensureEntryErrFmt, err)
	}
	if err := EnsureGitignoreEntry(root, "   "); err != nil {
		t.Fatalf(ensureEntryErrFmt, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if got := string(b); got != "node_modules\n" {
		t.Fatalf("expected unchanged content, got %q", got)
	}
}
