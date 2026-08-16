package projectfiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveProjectRoot returns the nearest enclosing Git worktree root. Outside
// a Git worktree, the canonical starting directory is the project root.
func ResolveProjectRoot(start string) (string, error) {
	start = strings.TrimSpace(start)
	if start == "" {
		var err error
		start, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	absolute, err := filepath.Abs(filepath.Clean(start))
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve project root links: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("inspect project root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project root is not a directory: %s", canonical)
	}

	for candidate := filepath.Clean(canonical); ; candidate = filepath.Dir(candidate) {
		if marker, markerErr := os.Lstat(filepath.Join(candidate, ".git")); markerErr == nil &&
			(marker.IsDir() || marker.Mode().IsRegular()) {
			return candidate, nil
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return filepath.Clean(canonical), nil
		}
	}
}
