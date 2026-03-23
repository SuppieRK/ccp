package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func verifyManagedRuleFile(target, missingFmt, guidanceFmt string, required []string) error {
	data, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf(missingFmt, target)
	}
	content := string(data)
	for _, needle := range required {
		if !strings.Contains(content, needle) {
			return fmt.Errorf(guidanceFmt, target)
		}
	}
	return nil
}

type ManagedRepoRuleFileAdapter struct {
	managedRuleFileAdapter
}

type ManagedHomeRuleFileAdapter struct {
	managedRuleFileAdapter
}

type managedRuleFileAdapter struct {
	id             string
	detectDir      string
	targetRelPath  string
	missingFmt     string
	guidanceFmt    string
	render         func() string
	verifyRequired []string
	resolveTarget  func(ctx Context, rel string) string
}

func NewManagedRepoRuleFileAdapter(
	id,
	detectDir,
	targetRelPath,
	missingFmt,
	guidanceFmt string,
	render func() string,
	verifyRequired []string,
) ManagedRepoRuleFileAdapter {
	return ManagedRepoRuleFileAdapter{
		managedRuleFileAdapter: managedRuleFileAdapter{
			id:             id,
			detectDir:      detectDir,
			targetRelPath:  targetRelPath,
			missingFmt:     missingFmt,
			guidanceFmt:    guidanceFmt,
			render:         render,
			verifyRequired: verifyRequired,
			resolveTarget: func(ctx Context, rel string) string {
				return ResolveRepoScopedPath(ctx.ScopeRoot, rel)
			},
		},
	}
}

func NewManagedHomeRuleFileAdapter(
	id,
	detectDir,
	targetRelPath,
	missingFmt,
	guidanceFmt string,
	render func() string,
	verifyRequired []string,
) ManagedHomeRuleFileAdapter {
	return ManagedHomeRuleFileAdapter{
		managedRuleFileAdapter: managedRuleFileAdapter{
			id:             id,
			detectDir:      detectDir,
			targetRelPath:  targetRelPath,
			missingFmt:     missingFmt,
			guidanceFmt:    guidanceFmt,
			render:         render,
			verifyRequired: verifyRequired,
			resolveTarget: func(ctx Context, rel string) string {
				return ResolveHomeScopedPath(ctx.HomeDir, rel)
			},
		},
	}
}

func (a managedRuleFileAdapter) ID() string { return a.id }

func (a managedRuleFileAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, a.detectDir)
}

func (a managedRuleFileAdapter) targetPath(ctx Context) string {
	return a.resolveTarget(ctx, a.targetRelPath)
}

func (a managedRuleFileAdapter) Plan(ctx Context) []PlannedArtifact {
	return []PlannedArtifact{{
		Kind:    ArtifactSettings,
		Path:    a.targetPath(ctx),
		Content: a.render(),
		Perm:    0o644,
	}}
}

func (a managedRuleFileAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	changed, err := write(a.targetPath(ctx), []byte(a.render()), 0o644)
	if err != nil {
		return InstallResult{}, err
	}
	if !changed {
		return InstallResult{Noop: 1}, nil
	}
	return InstallResult{Applied: 1}, nil
}

func (a managedRuleFileAdapter) Verify(ctx Context) error {
	return verifyManagedRuleFile(a.targetPath(ctx), a.missingFmt, a.guidanceFmt, a.verifyRequired)
}

func (a managedRuleFileAdapter) Uninstall(ctx Context) (InstallResult, error) {
	target := a.targetPath(ctx)
	removed, err := removeFileIfExists(target)
	if err != nil {
		return InstallResult{}, err
	}
	if !removed {
		return InstallResult{Noop: 1}, nil
	}
	return InstallResult{Applied: 1}, nil
}
