package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	codexManagedBlockStart = "<!-- BEGIN: CCP MANAGED BLOCK -->"
	codexManagedBlockEnd   = "<!-- END: CCP MANAGED BLOCK -->"
	codexAgentsPath        = ".codex/AGENTS.md"
)

type CodexAdapter struct{}

func NewCodexAdapter() CodexAdapter { return CodexAdapter{} }
func (a CodexAdapter) ID() string   { return string(AgentCodex) }

func (a CodexAdapter) DetectRoot(scopeRoot string) string {
	// Detection remains repo-root based while installation is global.
	return filepath.Join(scopeRoot, ".codex")
}

func (a CodexAdapter) Plan(ctx Context) []PlannedArtifact {
	return []PlannedArtifact{
		{
			Kind:    ArtifactSettings,
			Path:    ResolveHomeScopedPath(ctx.HomeDir, codexAgentsPath),
			Content: codexManagedBlockTemplate(),
			Perm:    0o644,
		},
	}
}

func (a CodexAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	target := ResolveHomeScopedPath(ctx.HomeDir, codexAgentsPath)
	content, err := upsertCodexManagedBlock(target)
	if err != nil {
		return InstallResult{}, err
	}
	changed, err := write(target, []byte(content), 0o644)
	if err != nil {
		return InstallResult{}, err
	}
	if !changed {
		return InstallResult{Noop: 1}, nil
	}
	return InstallResult{Applied: 1}, nil
}

func (a CodexAdapter) Verify(ctx Context) error {
	target := ResolveHomeScopedPath(ctx.HomeDir, codexAgentsPath)
	data, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("missing codex agents file: %s", target)
	}
	s := string(data)
	if !strings.Contains(s, codexManagedBlockStart) || !strings.Contains(s, codexManagedBlockEnd) {
		return fmt.Errorf("missing codex managed block markers in %s", target)
	}
	return nil
}

func (a CodexAdapter) Uninstall(ctx Context) (InstallResult, error) {
	target := ResolveHomeScopedPath(ctx.HomeDir, codexAgentsPath)
	updated, changed, removeAll, err := removeCodexManagedBlock(target)
	if err != nil {
		return InstallResult{}, err
	}
	if !changed {
		return InstallResult{Noop: 1}, nil
	}
	if removeAll {
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return InstallResult{}, err
		}
		return InstallResult{Applied: 1}, nil
	}
	if err := os.WriteFile(target, []byte(updated), 0o644); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{Applied: 1}, nil
}

func codexManagedBlockTemplate() string {
	return codexManagedBlockStart + "\n" +
		"## CCP Integration (Managed)\n\n" +
		"Use `ccp` as the command prefix for every executable in shell commands, including chained (`&&`, `||`) and piped (`|`) expressions.\n\n" +
		"Examples:\n" +
		"- `ccp ls -la`\n" +
		"- `ccp git status --short`\n" +
		"- `ccp go test -count=1 ./...`\n" +
		"- `ccp echo chain-ok && ccp echo chain-done`\n" +
		"- `ccp false || ccp echo chain-recovered`\n" +
		"- `ccp nl -ba spec.md | ccp sed -n '1,260p'`\n\n" +
		"If `ccp` is unavailable, run the original command and note that CCP is not installed.\n" +
		codexManagedBlockEnd + "\n"
}

func upsertCodexManagedBlock(path string) (string, error) {
	block := codexManagedBlockTemplate()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return block, nil
		}
		return "", err
	}
	existing := string(raw)

	start := strings.Index(existing, codexManagedBlockStart)
	end := strings.Index(existing, codexManagedBlockEnd)
	if start >= 0 && end >= start {
		end += len(codexManagedBlockEnd)
		tailStart := skipSingleLF(existing, end)
		updated := existing[:start] + strings.TrimRight(block, "\n") + "\n" + existing[tailStart:]
		return normalizeManagedFile(updated), nil
	}

	trimmed := strings.TrimRight(existing, "\n")
	if trimmed == "" {
		return block, nil
	}
	return normalizeManagedFile(trimmed + "\n\n" + block), nil
}

func normalizeManagedFile(in string) string {
	out := strings.ReplaceAll(in, "\r\n", "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}

func removeCodexManagedBlock(path string) (updated string, changed bool, removeAll bool, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, false, nil
		}
		return "", false, false, err
	}
	existing := string(raw)
	start := strings.Index(existing, codexManagedBlockStart)
	end := strings.Index(existing, codexManagedBlockEnd)
	if start < 0 || end < start {
		return "", false, false, nil
	}
	end += len(codexManagedBlockEnd)
	tailStart := skipSingleLF(existing, end)
	merged := strings.TrimSpace(existing[:start] + existing[tailStart:])
	if merged == "" {
		return "", true, true, nil
	}
	return normalizeManagedFile(merged), true, false, nil
}

func skipSingleLF(s string, idx int) int {
	if idx < len(s) && s[idx] == '\n' {
		return idx + 1
	}
	return idx
}
