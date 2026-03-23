package agents

import (
	"fmt"
	"os"
	"strings"
)

const (
	ccpManagedBlockStart = "<!-- BEGIN: CCP MANAGED BLOCK -->"
	ccpManagedBlockEnd   = "<!-- END: CCP MANAGED BLOCK -->"
	ccpRawEscapeHatch    = "If output seems corrupted, malformed, or unusable for the task, retry the command with `ccp --raw` as an escape hatch."
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

func verifyManagedContextBlock(target, missingFmt, markersFmt string) error {
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

func upsertManagedContextBlock(path string) (string, error) {
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

func removeManagedContextBlock(path string) (updated string, changed bool, removeAll bool, err error) {
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

type ManagedContextFileAdapter struct {
	spec ManagedContextFileAdapterSpec
}

type managedContextTargetScope string

const (
	managedContextTargetHome managedContextTargetScope = "home"
	managedContextTargetRepo managedContextTargetScope = "repo"
)

type ManagedContextFileAdapterSpec struct {
	ID             ID
	DetectRootPath string
	TargetRelPath  string
	TargetScope    managedContextTargetScope
	MissingFmt     string
	MarkersFmt     string
}

type ManagedContextAdapter = ManagedContextFileAdapter

type ManagedRepoContextFileAdapter = ManagedContextFileAdapter

func NewManagedContextAdapter(spec ManagedContextFileAdapterSpec) ManagedContextAdapter {
	return ManagedContextAdapter{spec: spec}
}

func NewManagedContextFileAdapter(id, detectDir, targetRelPath, missingFmt, markersFmt string) ManagedContextFileAdapter {
	return NewManagedContextAdapter(ManagedContextFileAdapterSpec{
		ID:             ID(id),
		DetectRootPath: detectDir,
		TargetRelPath:  targetRelPath,
		TargetScope:    managedContextTargetHome,
		MissingFmt:     missingFmt,
		MarkersFmt:     markersFmt,
	})
}

func (a ManagedContextFileAdapter) ID() string { return string(a.spec.ID) }

func (a ManagedContextFileAdapter) DetectRoot(scopeRoot string) string {
	return ResolveRepoScopedPath(scopeRoot, a.spec.DetectRootPath)
}

func (a ManagedContextFileAdapter) targetPath(ctx Context) string {
	switch a.spec.TargetScope {
	case managedContextTargetRepo:
		return ResolveRepoScopedPath(ctx.ScopeRoot, a.spec.TargetRelPath)
	default:
		return ResolveHomeScopedPath(ctx.HomeDir, a.spec.TargetRelPath)
	}
}

func (a ManagedContextFileAdapter) Plan(ctx Context) []PlannedArtifact {
	return []PlannedArtifact{{
		Kind:    ArtifactSettings,
		Path:    a.targetPath(ctx),
		Content: ccpManagedBlockTemplate(),
		Perm:    0o644,
	}}
}

func (a ManagedContextFileAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	target := a.targetPath(ctx)
	content, err := upsertManagedContextBlock(target)
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

func (a ManagedContextFileAdapter) Verify(ctx Context) error {
	return verifyManagedContextBlock(a.targetPath(ctx), a.spec.MissingFmt, a.spec.MarkersFmt)
}

func (a ManagedContextFileAdapter) Uninstall(ctx Context) (InstallResult, error) {
	target := a.targetPath(ctx)
	updated, changed, removeAll, err := removeManagedContextBlock(target)
	if err != nil {
		return InstallResult{}, err
	}
	if !changed {
		return InstallResult{Noop: 1}, nil
	}
	return applyManagedFileChange(target, updated, changed, removeAll)
}
