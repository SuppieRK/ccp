package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ccpManagedBlockStart = "<!-- BEGIN: CCP MANAGED BLOCK -->"
	ccpManagedBlockEnd   = "<!-- END: CCP MANAGED BLOCK -->"
	ccpRawEscapeHatch    = "If output seems corrupted or unclear, retry the command with `ccp --raw` as an escape hatch."
)

func ccpManagedBlockTemplate() string {
	return ccpManagedBlockStart + "\n" + ccpManagedGuidanceMarkdown() + ccpManagedBlockEnd + "\n"
}

func ccpManagedGuidanceMarkdown() string {
	return "## CCP Integration (Managed)\n\n" +
		"Use `ccp` as the command prefix for every executable in shell commands, including chained (`&&`, `||`) and piped (`|`) expressions.\n\n" +
		"Examples:\n" +
		"- `ccp ls -la`\n" +
		"- `ccp git status --short`\n" +
		"- `ccp go test -count=1 ./...`\n" +
		"- `ccp echo chain-ok && ccp echo chain-done`\n" +
		"- `ccp false || ccp echo chain-recovered`\n" +
		"- `ccp nl -ba spec.md | ccp sed -n '1,260p'`\n\n" +
		ccpRawEscapeHatch + "\n\n" +
		"If `ccp` is unavailable, run the original command and note that CCP is not installed.\n"
}

func verifyManagedInstructionBlock(target, missingFmt, markersFmt string) error {
	data, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf(missingFmt, target)
	}
	s := string(data)
	if !strings.Contains(s, ccpManagedBlockStart) || !strings.Contains(s, ccpManagedBlockEnd) {
		return fmt.Errorf(markersFmt, target)
	}
	if !strings.Contains(s, ccpRawEscapeHatch) {
		return fmt.Errorf(markersFmt, target)
	}
	return nil
}

func upsertManagedInstructionBlock(path string) (string, error) {
	block := ccpManagedBlockTemplate()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return block, nil
		}
		return "", err
	}
	existing := string(raw)

	start := strings.Index(existing, ccpManagedBlockStart)
	end := strings.Index(existing, ccpManagedBlockEnd)
	if start >= 0 && end >= start {
		end += len(ccpManagedBlockEnd)
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

func removeManagedInstructionBlock(path string) (updated string, changed bool, removeAll bool, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, false, nil
		}
		return "", false, false, err
	}
	existing := string(raw)
	start := strings.Index(existing, ccpManagedBlockStart)
	end := strings.Index(existing, ccpManagedBlockEnd)
	if start < 0 || end < start {
		return "", false, false, nil
	}
	end += len(ccpManagedBlockEnd)
	tailStart := skipSingleLF(existing, end)
	merged := strings.TrimSpace(existing[:start] + existing[tailStart:])
	if merged == "" {
		return "", true, true, nil
	}
	return normalizeManagedFile(merged), true, false, nil
}

func normalizeManagedFile(in string) string {
	out := strings.ReplaceAll(in, "\r\n", "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}

func skipSingleLF(s string, idx int) int {
	if idx < len(s) && s[idx] == '\n' {
		return idx + 1
	}
	return idx
}

type ManagedInstructionFileAdapter struct {
	id            string
	detectDir     string
	targetRelPath string
	missingFmt    string
	markersFmt    string
}

func NewManagedInstructionFileAdapter(id, detectDir, targetRelPath, missingFmt, markersFmt string) ManagedInstructionFileAdapter {
	return ManagedInstructionFileAdapter{
		id:            id,
		detectDir:     detectDir,
		targetRelPath: targetRelPath,
		missingFmt:    missingFmt,
		markersFmt:    markersFmt,
	}
}

func (a ManagedInstructionFileAdapter) ID() string { return a.id }

func (a ManagedInstructionFileAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, a.detectDir)
}

func (a ManagedInstructionFileAdapter) targetPath(ctx Context) string {
	return ResolveHomeScopedPath(ctx.HomeDir, a.targetRelPath)
}

func (a ManagedInstructionFileAdapter) Plan(ctx Context) []PlannedArtifact {
	return []PlannedArtifact{{
		Kind:    ArtifactSettings,
		Path:    a.targetPath(ctx),
		Content: ccpManagedBlockTemplate(),
		Perm:    0o644,
	}}
}

func (a ManagedInstructionFileAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	target := a.targetPath(ctx)
	content, err := upsertManagedInstructionBlock(target)
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

func (a ManagedInstructionFileAdapter) Verify(ctx Context) error {
	return verifyManagedInstructionBlock(a.targetPath(ctx), a.missingFmt, a.markersFmt)
}

func (a ManagedInstructionFileAdapter) Uninstall(ctx Context) (InstallResult, error) {
	target := a.targetPath(ctx)
	updated, changed, removeAll, err := removeManagedInstructionBlock(target)
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
