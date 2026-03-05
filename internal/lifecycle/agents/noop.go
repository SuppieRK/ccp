package agents

import "path/filepath"

// NoopAdapter represents a recognized-but-not-yet-implemented integration.
// It intentionally performs no filesystem mutations.
type NoopAdapter struct {
	id      string
	rootDir string
}

func NewNoopAdapter(id, rootDir string) NoopAdapter { return NoopAdapter{id: id, rootDir: rootDir} }

func (a NoopAdapter) ID() string                         { return a.id }
func (a NoopAdapter) DetectRoot(scopeRoot string) string { return filepath.Join(scopeRoot, a.rootDir) }

func (a NoopAdapter) Install(_ Context, _ WriterFunc) (InstallResult, error) {
	return InstallResult{Noop: 1}, nil
}

func (a NoopAdapter) Plan(_ Context) []PlannedArtifact { return nil }
func (a NoopAdapter) Verify(_ Context) error           { return nil }

func (a NoopAdapter) Uninstall(_ Context) (InstallResult, error) { return InstallResult{Noop: 1}, nil }
