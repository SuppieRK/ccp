//go:build !windows

package projectfiles

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func openFileBeneath(root, relative string, flag int, perm os.FileMode) (*os.File, error) {
	parentFD, base, err := openContainedParent(root, relative, true)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unix.Close(parentFD) }()
	beforeContainedFinalOpen()

	fd, err := unix.Openat(parentFD, base, flag|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(perm.Perm()))
	if err != nil {
		return nil, fmt.Errorf("open contained file %q: %w", relative, err)
	}
	if err := validateContainedFileFD(fd, relative); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), filepath.Join(root, relative)), nil
}

func atomicWriteFileBeneath(root, relative string, data []byte, perm os.FileMode) (retErr error) {
	parentFD, base, err := openContainedParent(root, relative, true)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(parentFD) }()

	finalMode, err := containedDestinationMode(parentFD, base, perm, relative)
	if err != nil {
		return err
	}
	tmpName, fd, err := createContainedTemporary(parentFD, base)
	if err != nil {
		return err
	}
	tmpExists := true
	file := os.NewFile(uintptr(fd), filepath.Join(root, filepath.Dir(relative), tmpName))
	defer func() {
		retErr = errors.Join(retErr, cleanupContainedTemporary(file, parentFD, tmpName, tmpExists))
	}()

	if err := prepareContainedTemporary(file, data, finalMode); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		file = nil
		return fmt.Errorf("close contained temporary file: %w", err)
	}
	file = nil

	if err := unix.Renameat(parentFD, tmpName, parentFD, base); err != nil {
		return fmt.Errorf("replace contained file %q: %w", relative, err)
	}
	tmpExists = false
	if err := unix.Fsync(parentFD); err != nil {
		return fmt.Errorf("sync contained parent for %q: %w", relative, err)
	}
	return nil
}

func cleanupContainedTemporary(file *os.File, parentFD int, name string, exists bool) error {
	var cleanupErr error
	if file != nil {
		cleanupErr = file.Close()
	}
	if !exists {
		return cleanupErr
	}
	removeErr := unix.Unlinkat(parentFD, name, 0)
	if removeErr != nil && !errors.Is(removeErr, unix.ENOENT) {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove contained temporary file: %w", removeErr))
	}
	return cleanupErr
}

func prepareContainedTemporary(file *os.File, data []byte, mode os.FileMode) error {
	if err := file.Chmod(mode); err != nil {
		return fmt.Errorf("set contained temporary file mode: %w", err)
	}
	if err := writeAtomicBytes(file, data); err != nil {
		return fmt.Errorf("write contained temporary file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync contained temporary file: %w", err)
	}
	return nil
}

func openContainedParent(root, relative string, create bool) (int, string, error) {
	parts, err := containedPathParts(relative)
	if err != nil {
		return -1, "", err
	}

	current, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, "", fmt.Errorf("open contained root %q: %w", root, err)
	}
	for _, part := range parts[:len(parts)-1] {
		next, openErr := openContainedDirectory(current, part, create)
		if openErr != nil {
			_ = unix.Close(current)
			return -1, "", openErr
		}
		if err := unix.Close(current); err != nil {
			_ = unix.Close(next)
			return -1, "", err
		}
		current = next
	}
	return current, parts[len(parts)-1], nil
}

func containedPathParts(relative string) ([]string, error) {
	parts := make([]string, 0, 8)
	for part := range strings.SplitSeq(filepath.Clean(relative), string(os.PathSeparator)) {
		if part == "" || part == "." || part == ".." {
			return nil, fmt.Errorf("invalid contained path component %q", part)
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return nil, errors.New("contained path has no file component")
	}
	return parts, nil
}

func openContainedDirectory(parentFD int, part string, create bool) (int, error) {
	next, err := unix.Openat(parentFD, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if !errors.Is(err, unix.ENOENT) || !create {
		if err != nil {
			return -1, fmt.Errorf("open contained directory %q: %w", part, err)
		}
		return next, nil
	}
	if mkdirErr := unix.Mkdirat(parentFD, part, 0o755); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
		return -1, fmt.Errorf("create contained directory %q: %w", part, mkdirErr)
	}
	next, err = unix.Openat(parentFD, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, fmt.Errorf("open contained directory %q: %w", part, err)
	}
	return next, nil
}

func validateContainedFileFD(fd int, relative string) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fmt.Errorf("inspect contained file %q: %w", relative, err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return fmt.Errorf("refuse non-regular contained file %q", relative)
	}
	if stat.Nlink > 1 {
		return fmt.Errorf("refuse hard-linked contained file %q", relative)
	}
	return nil
}

func containedDestinationMode(parentFD int, base string, requested os.FileMode, relative string) (os.FileMode, error) {
	var stat unix.Stat_t
	err := unix.Fstatat(parentFD, base, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, unix.ENOENT) {
		return requested.Perm(), nil
	}
	if err != nil {
		return 0, fmt.Errorf("inspect contained destination %q: %w", relative, err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return 0, fmt.Errorf("refuse non-regular contained file %q", relative)
	}
	if stat.Nlink > 1 {
		return 0, fmt.Errorf("refuse hard-linked contained file %q", relative)
	}
	return os.FileMode(stat.Mode).Perm(), nil
}

func createContainedTemporary(parentFD int, base string) (string, int, error) {
	for range 100 {
		var suffix [12]byte
		if _, err := io.ReadFull(rand.Reader, suffix[:]); err != nil {
			return "", -1, fmt.Errorf("generate contained temporary name: %w", err)
		}
		name := "." + base + ".tmp-" + hex.EncodeToString(suffix[:])
		fd, err := unix.Openat(
			parentFD,
			name,
			unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,
			0o600,
		)
		if err == nil {
			return name, fd, nil
		}
		if !errors.Is(err, unix.EEXIST) {
			return "", -1, fmt.Errorf("create contained temporary file: %w", err)
		}
	}
	return "", -1, errors.New("create contained temporary file: exhausted name attempts")
}
