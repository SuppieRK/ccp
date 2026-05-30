package agents

import "path/filepath"

const (
	piDetectRootPath       = ".pi"
	piAppendSystemPath     = ".pi/APPEND_SYSTEM.md"
	piLegacyAgentsRootPath = "AGENTS.md"
)

type PiAdapter struct{}

func (PiAdapter) ID() string { return string(AgentPi) }

func (PiAdapter) DetectRoot(scopeRoot string) string {
	return ResolveRepoScopedPath(scopeRoot, piDetectRootPath)
}

func (PiAdapter) appendSystemPath(ctx Context) string {
	return ResolveRepoScopedPath(ctx.ScopeRoot, piAppendSystemPath)
}

func (PiAdapter) legacyAgentsPath(ctx Context) string {
	return ResolveRepoScopedPath(ctx.ScopeRoot, piLegacyAgentsRootPath)
}

func (a PiAdapter) Plan(ctx Context) []PlannedArtifact {
	return []PlannedArtifact{{
		Kind:    ArtifactSettings,
		Path:    a.appendSystemPath(ctx),
		Content: ccpManagedBlockTemplate(),
		Perm:    0o644,
	}}
}

func (a PiAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	target := a.appendSystemPath(ctx)
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

func (a PiAdapter) Verify(ctx Context) error {
	return verifyManagedContextBlock(a.appendSystemPath(ctx), "missing pi append system file: %s", "missing pi managed block markers in %s")
}

func (a PiAdapter) Uninstall(ctx Context) (InstallResult, error) {
	var total InstallResult
	for _, target := range []string{a.appendSystemPath(ctx), a.legacyAgentsPath(ctx)} {
		updated, changed, removeAll, err := removeManagedContextBlock(filepath.Clean(target))
		if err != nil {
			return InstallResult{}, err
		}
		res, err := applyManagedFileChange(target, updated, changed, removeAll)
		if err != nil {
			return InstallResult{}, err
		}
		total.Applied += res.Applied
		total.Noop += res.Noop
	}
	return total, nil
}
