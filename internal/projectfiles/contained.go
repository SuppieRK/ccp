package projectfiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var beforeContainedFinalOpen = func() {}

// OpenFileBeneath opens path while requiring every resolved component to stay
// beneath root and rejecting link-like traversal.
func OpenFileBeneath(root, path string, flag int, perm os.FileMode) (*os.File, error) {
	canonicalRoot, relative, err := containedPath(root, path)
	if err != nil {
		return nil, err
	}
	return openFileBeneath(canonicalRoot, relative, flag, perm)
}

// AtomicWriteFileBeneath atomically replaces path while requiring it to remain
// beneath root throughout the operation.
func AtomicWriteFileBeneath(root, path string, data []byte, perm os.FileMode) error {
	canonicalRoot, relative, err := containedPath(root, path)
	if err != nil {
		return err
	}
	return atomicWriteFileBeneath(canonicalRoot, relative, data, perm)
}

// CanonicalPathBeneath returns the canonical contained path without following
// or requiring the final path to exist.
func CanonicalPathBeneath(root, path string) (string, error) {
	canonicalRoot, relative, err := containedPath(root, path)
	if err != nil {
		return "", err
	}
	return filepath.Join(canonicalRoot, relative), nil
}

// ValidateRegularFileBeneath confirms that path currently resolves to a
// regular, non-linked file beneath root using the same safe opener as writes.
func ValidateRegularFileBeneath(root, path string) error {
	file, err := OpenFileBeneath(root, path, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	return file.Close()
}

func containedPath(root, path string) (string, string, error) {
	root = strings.TrimSpace(root)
	path = strings.TrimSpace(path)
	if root == "" || path == "" {
		return "", "", fmt.Errorf("contained root and path must not be empty")
	}
	absRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", "", fmt.Errorf("resolve contained root: %w", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", "", fmt.Errorf("resolve contained root links: %w", err)
	}
	canonicalRoot = filepath.Clean(canonicalRoot)

	absPath := path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(absRoot, absPath)
	}
	absPath, err = filepath.Abs(filepath.Clean(absPath))
	if err != nil {
		return "", "", fmt.Errorf("resolve contained path: %w", err)
	}

	// Darwin exposes temporary directories through /var while resolving them
	// to /private/var, and Windows can expose the same directory through both
	// DOS 8.3 and long path spellings. Prefer the caller's lexical root so both
	// sides stay in the same namespace, then accept an already canonical path.
	relative, contained, relativeErr := relativePathBeneath(absRoot, absPath)
	if !contained && canonicalRoot != absRoot {
		var canonicalErr error
		relative, contained, canonicalErr = relativePathBeneath(canonicalRoot, absPath)
		if relativeErr == nil {
			relativeErr = canonicalErr
		}
	}
	if !contained {
		if relativeErr != nil {
			return "", "", relativeErr
		}
		return "", "", fmt.Errorf("refuse path %q outside contained root %q", path, canonicalRoot)
	}
	return canonicalRoot, relative, nil
}

func relativePathBeneath(root, path string) (string, bool, error) {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return "", false, fmt.Errorf("relativize contained path: %w", err)
	}
	relative = filepath.Clean(relative)
	if relative == "." || relative == "" || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
		return "", false, nil
	}
	return relative, true, nil
}
