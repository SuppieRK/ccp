package agents

import (
	"fmt"
	"os"
	"strings"
)

const (
	cmdshapeManagedBlockStart = "<!-- BEGIN: CMDSHAPE MANAGED BLOCK -->"
	cmdshapeManagedBlockEnd   = "<!-- END: CMDSHAPE MANAGED BLOCK -->"
	legacyManagedBlockStart   = "<!-- BEGIN: CCP MANAGED BLOCK -->"
	legacyManagedBlockEnd     = "<!-- END: CCP MANAGED BLOCK -->"
	cmdshapeRawEscapeHatch    = "If output seems corrupted, malformed, or unusable for the task, retry the command with `cmdshape --raw` as an escape hatch."
	cmdshapeFilterPromptHint  = "When asked to create or improve cmdshape YAML filters, run `cmdshape filter prompt` for the embedded self-service workflow."
)

func cmdshapeManagedBlockTemplate() string {
	return cmdshapeManagedBlockStart + "\n" + cmdshapeManagedGuidanceMarkdown() + cmdshapeManagedBlockEnd + "\n"
}

func cmdshapeManagedGuidanceMarkdown() string {
	return "## cmdshape Integration (Managed)\n\n" +
		"Use `cmdshape` as the command prefix for every executable in shell commands, including chained (`&&`, `||`) and piped (`|`) expressions.\n\n" +
		"Examples:\n" +
		"- `cmdshape ls -la`\n" +
		"- `cmdshape git status --short`\n" +
		"- `cmdshape go test -count=1 ./...`\n" +
		"- `cmdshape echo chain-ok && cmdshape echo chain-done`\n" +
		"- `cmdshape false || cmdshape echo chain-recovered`\n" +
		"- `cmdshape nl -ba spec.md | cmdshape sed -n '1,260p'`\n\n" +
		cmdshapeRawEscapeHatch + "\n\n" +
		cmdshapeFilterPromptHint + "\n\n" +
		"If `cmdshape` is unavailable, run the original command and note that cmdshape is not installed.\n"
}

func verifyManagedContextBlock(target, missingFmt, markersFmt string) error {
	data, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf(missingFmt, target)
	}
	s := string(data)
	if !strings.Contains(s, cmdshapeManagedBlockStart) || !strings.Contains(s, cmdshapeManagedBlockEnd) {
		return fmt.Errorf(markersFmt, target)
	}
	if !strings.Contains(s, cmdshapeRawEscapeHatch) {
		return fmt.Errorf(markersFmt, target)
	}
	if !strings.Contains(s, cmdshapeFilterPromptHint) {
		return fmt.Errorf(markersFmt, target)
	}
	return nil
}

func upsertManagedContextBlock(path string) (string, error) {
	block := cmdshapeManagedBlockTemplate()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return block, nil
		}
		return "", err
	}
	existing := string(raw)

	start, end, found := managedBlockBounds(existing)
	if found {
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
	start, end, found := managedBlockBounds(existing)
	if !found {
		return "", false, false, nil
	}
	tailStart := skipSingleLF(existing, end)
	merged := strings.TrimSpace(existing[:start] + existing[tailStart:])
	if merged == "" {
		return "", true, true, nil
	}
	return normalizeManagedFile(merged), true, false, nil
}

func managedBlockBounds(existing string) (int, int, bool) {
	for _, markers := range [][2]string{
		{cmdshapeManagedBlockStart, cmdshapeManagedBlockEnd},
		{legacyManagedBlockStart, legacyManagedBlockEnd},
	} {
		start := strings.Index(existing, markers[0])
		end := strings.Index(existing, markers[1])
		if start < 0 || end < start {
			continue
		}
		return start, end + len(markers[1]), true
	}
	return 0, 0, false
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
		Content: cmdshapeManagedBlockTemplate(),
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
