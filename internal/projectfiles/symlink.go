package projectfiles

import (
	"fmt"
	"os"
	"path/filepath"
)

func RejectSymlinkPath(path string) error {
	resolved, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	for current := resolved; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("refuse to use symlink path component %q", current)
			}
			if info.IsDir() {
				return nil
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
	}
}
