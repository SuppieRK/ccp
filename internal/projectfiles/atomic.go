package projectfiles

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type atomicFile interface {
	io.Writer
	Chmod(os.FileMode) error
	Close() error
	Name() string
	Sync() error
}

type atomicWriteOps struct {
	lstat      func(string) (os.FileInfo, error)
	createTemp func(string, string) (atomicFile, error)
	remove     func(string) error
	replace    func(string, string) error
	syncDir    func(string) error
}

var defaultAtomicWriteOps = atomicWriteOps{
	lstat: os.Lstat,
	createTemp: func(dir, pattern string) (atomicFile, error) {
		return os.CreateTemp(dir, pattern)
	},
	remove:  os.Remove,
	replace: replaceFile,
	syncDir: syncDirectory,
}

// AtomicWriteFile replaces path with data without exposing a partially written
// destination. Existing destination permissions are preserved.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	return atomicWriteFile(path, data, perm, defaultAtomicWriteOps)
}

func atomicWriteFile(path string, data []byte, perm os.FileMode, ops atomicWriteOps) (retErr error) {
	if err := RejectSymlinkPath(path); err != nil {
		return err
	}

	finalMode, err := atomicDestinationMode(path, perm, ops.lstat)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := ops.createTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %q: %w", path, err)
	}
	tmpPath := tmp.Name()
	replaced := false
	defer func() {
		retErr = errors.Join(retErr, cleanupAtomicTemporary(tmp, tmpPath, replaced, ops.remove))
	}()

	if err := prepareAtomicTemporary(tmp, data, finalMode, path); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		tmp = nil
		return fmt.Errorf("close temporary file for %q: %w", path, err)
	}
	tmp = nil

	if err := RejectSymlinkPath(path); err != nil {
		return err
	}
	if err := ops.replace(tmpPath, path); err != nil {
		return fmt.Errorf("replace %q atomically: %w", path, err)
	}
	replaced = true
	if err := ops.syncDir(dir); err != nil {
		return fmt.Errorf("sync parent directory for %q: %w", path, err)
	}
	return nil
}

func cleanupAtomicTemporary(tmp atomicFile, path string, replaced bool, remove func(string) error) error {
	var cleanupErr error
	if tmp != nil {
		cleanupErr = tmp.Close()
	}
	if replaced {
		return cleanupErr
	}
	removeErr := remove(path)
	if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove temporary file %q: %w", path, removeErr))
	}
	return cleanupErr
}

func prepareAtomicTemporary(tmp atomicFile, data []byte, mode os.FileMode, destination string) error {
	if err := tmp.Chmod(mode); err != nil {
		return fmt.Errorf("set temporary file mode for %q: %w", destination, err)
	}
	if err := writeAtomicBytes(tmp, data); err != nil {
		return fmt.Errorf("write temporary file for %q: %w", destination, err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temporary file for %q: %w", destination, err)
	}
	return nil
}

func atomicDestinationMode(path string, requested os.FileMode, lstat func(string) (os.FileInfo, error)) (os.FileMode, error) {
	info, err := lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return 0, fmt.Errorf("refuse to replace non-regular file %q", path)
		}
		return info.Mode().Perm(), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("inspect destination %q: %w", path, err)
	}
	return requested.Perm(), nil
}

func writeAtomicBytes(dst io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := dst.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}
