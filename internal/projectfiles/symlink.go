package projectfiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func RejectSymlinkPath(path string) error {
	resolved, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	resolved = filepath.Clean(resolved)
	volume := filepath.VolumeName(resolved)
	remainder := strings.TrimPrefix(resolved[len(volume):], string(os.PathSeparator))
	current := volume + string(os.PathSeparator)
	for part := range strings.SplitSeq(remainder, string(os.PathSeparator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		parent := filepath.Dir(current)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// Allow top-level system aliases such as macOS `/var -> /private/var`.
			if parent == volume+string(os.PathSeparator) {
				continue
			}
			return fmt.Errorf("refuse to use symlink path component %q", current)
		}
	}
	return nil
}
