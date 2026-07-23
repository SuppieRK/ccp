//go:build !windows

package projectfiles

import (
	"errors"
	"os"
)

func replaceFile(src, dst string) error {
	return os.Rename(src, dst)
}

func syncDirectory(path string) (retErr error) {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		retErr = errors.Join(retErr, dir.Close())
	}()
	return dir.Sync()
}
