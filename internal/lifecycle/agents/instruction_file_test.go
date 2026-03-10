package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestManagedInstructionBlockHelpers(t *testing.T) {
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

func TestManagedInstructionBlockUpsertBranches(t *testing.T) {
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

func TestManagedInstructionBlockUpsertOnMissingFileUsesCanonicalTemplate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "missing", agentsFileName)
	out, err := upsertManagedInstructionBlock(p)
	if err != nil {
		t.Fatalf("upsert missing file: %v", err)
	}
	if out != ccpManagedBlockTemplate() {
		t.Fatalf("expected canonical template for missing file, got %q", out)
	}
}
