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
	id             string
	detectDir      string
	targetRelPath  string
	missingFmt     string
	guidanceFmt    string
	render         func() string
	verifyRequired []string
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
		id:             id,
		detectDir:      detectDir,
		targetRelPath:  targetRelPath,
		missingFmt:     missingFmt,
		guidanceFmt:    guidanceFmt,
		render:         render,
		verifyRequired: verifyRequired,
	}
}

func (a ManagedRepoRuleFileAdapter) ID() string { return a.id }

func (a ManagedRepoRuleFileAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, a.detectDir)
}

func (a ManagedRepoRuleFileAdapter) targetPath(ctx Context) string {
	return filepath.Join(ctx.ScopeRoot, a.targetRelPath)
}

func (a ManagedRepoRuleFileAdapter) Plan(ctx Context) []PlannedArtifact {
	return []PlannedArtifact{{
		Kind:    ArtifactSettings,
		Path:    a.targetPath(ctx),
		Content: a.render(),
		Perm:    0o644,
	}}
}

func (a ManagedRepoRuleFileAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	changed, err := write(a.targetPath(ctx), []byte(a.render()), 0o644)
	if err != nil {
		return InstallResult{}, err
	}
	if !changed {
		return InstallResult{Noop: 1}, nil
	}
	return InstallResult{Applied: 1}, nil
}

func (a ManagedRepoRuleFileAdapter) Verify(ctx Context) error {
	return verifyManagedRuleFile(a.targetPath(ctx), a.missingFmt, a.guidanceFmt, a.verifyRequired)
}

func (a ManagedRepoRuleFileAdapter) Uninstall(ctx Context) (InstallResult, error) {
	target := a.targetPath(ctx)
	if err := os.Remove(target); err != nil {
		if os.IsNotExist(err) {
			return InstallResult{Noop: 1}, nil
		}
		return InstallResult{}, err
	}
	return InstallResult{Applied: 1}, nil
}
