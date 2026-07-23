//go:build windows

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
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsShareAll = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE

type windowsFileRenameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

func openFileBeneath(root, relative string, flag int, perm os.FileMode) (*os.File, error) {
	parent, base, err := openWindowsContainedParent(root, relative)
	if err != nil {
		return nil, err
	}
	defer func() { _ = windows.CloseHandle(parent) }()
	beforeContainedFinalOpen()

	handle, err := openWindowsRelative(parent, base, flag, windows.FILE_NON_DIRECTORY_FILE)
	if err != nil {
		return nil, fmt.Errorf("open contained file %q: %w", relative, err)
	}
	file := os.NewFile(uintptr(handle), filepath.Join(root, relative))
	if err := validateWindowsContainedHandle(handle, relative, false); err != nil {
		_ = file.Close()
		return nil, err
	}
	if flag&os.O_CREATE != 0 {
		if err := file.Chmod(perm.Perm()); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("set contained file mode: %w", err)
		}
	}
	return file, nil
}

func atomicWriteFileBeneath(root, relative string, data []byte, perm os.FileMode) (retErr error) {
	parent, base, err := openWindowsContainedParent(root, relative)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(parent) }()

	finalMode, err := windowsContainedDestinationMode(parent, base, perm, relative)
	if err != nil {
		return err
	}
	tmpName, handle, err := createWindowsContainedTemporary(parent, base)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), filepath.Join(root, filepath.Dir(relative), tmpName))
	tmpExists := true
	defer func() {
		if file != nil {
			retErr = errors.Join(retErr, file.Close())
		}
		if tmpExists {
			retErr = errors.Join(retErr, removeWindowsRelative(parent, tmpName))
		}
	}()

	if err := file.Chmod(finalMode); err != nil {
		return fmt.Errorf("set contained temporary file mode: %w", err)
	}
	if err := writeAtomicBytes(file, data); err != nil {
		return fmt.Errorf("write contained temporary file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync contained temporary file: %w", err)
	}
	if err := renameWindowsRelative(handle, parent, base); err != nil {
		return fmt.Errorf("replace contained file %q: %w", relative, err)
	}
	tmpExists = false
	if err := file.Close(); err != nil {
		file = nil
		return fmt.Errorf("close contained temporary file: %w", err)
	}
	file = nil
	return nil
}

func openWindowsContainedParent(root, relative string) (windows.Handle, string, error) {
	parts := make([]string, 0, 8)
	for part := range strings.SplitSeq(filepath.Clean(relative), string(os.PathSeparator)) {
		if part == "" || part == "." || part == ".." {
			return windows.InvalidHandle, "", fmt.Errorf("invalid contained path component %q", part)
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return windows.InvalidHandle, "", fmt.Errorf("contained path has no file component")
	}

	rootPath, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return windows.InvalidHandle, "", err
	}
	current, err := windows.CreateFile(
		rootPath,
		windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE,
		windowsShareAll,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return windows.InvalidHandle, "", fmt.Errorf("open contained root %q: %w", root, err)
	}
	if err := validateWindowsContainedHandle(current, root, true); err != nil {
		_ = windows.CloseHandle(current)
		return windows.InvalidHandle, "", err
	}

	for _, part := range parts[:len(parts)-1] {
		next, openErr := openWindowsDirectoryRelative(current, part)
		if openErr != nil {
			_ = windows.CloseHandle(current)
			return windows.InvalidHandle, "", fmt.Errorf("open contained directory %q: %w", part, openErr)
		}
		if err := validateWindowsContainedHandle(next, part, true); err != nil {
			_ = windows.CloseHandle(next)
			_ = windows.CloseHandle(current)
			return windows.InvalidHandle, "", err
		}
		if err := windows.CloseHandle(current); err != nil {
			_ = windows.CloseHandle(next)
			return windows.InvalidHandle, "", err
		}
		current = next
	}
	return current, parts[len(parts)-1], nil
}

func openWindowsDirectoryRelative(parent windows.Handle, name string) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return windows.InvalidHandle, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		RootDirectory: parent,
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	attributes.Length = uint32(unsafe.Sizeof(*attributes))
	var (
		handle windows.Handle
		status windows.IO_STATUS_BLOCK
		size   int64
	)
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE,
		attributes,
		&status,
		&size,
		windows.FILE_ATTRIBUTE_NORMAL,
		windowsShareAll,
		windows.FILE_OPEN_IF,
		windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT,
		0,
		0,
	)
	return handle, err
}

func openWindowsRelative(parent windows.Handle, name string, flag int, options uint32) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return windows.InvalidHandle, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		RootDirectory: parent,
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	attributes.Length = uint32(unsafe.Sizeof(*attributes))

	access := uint32(windows.FILE_GENERIC_READ)
	switch flag & (os.O_WRONLY | os.O_RDWR) {
	case os.O_WRONLY:
		access = windows.FILE_GENERIC_WRITE
	case os.O_RDWR:
		access = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE
	}
	if flag&os.O_APPEND != 0 {
		access |= windows.FILE_APPEND_DATA
	}
	if flag&os.O_EXCL != 0 {
		access |= windows.DELETE
	}

	disposition := uint32(windows.FILE_OPEN)
	switch {
	case flag&os.O_CREATE != 0 && flag&os.O_EXCL != 0:
		disposition = windows.FILE_CREATE
	case flag&os.O_CREATE != 0 && flag&os.O_TRUNC != 0:
		disposition = windows.FILE_OVERWRITE_IF
	case flag&os.O_CREATE != 0:
		disposition = windows.FILE_OPEN_IF
	case flag&os.O_TRUNC != 0:
		disposition = windows.FILE_OVERWRITE
	}
	var (
		handle windows.Handle
		status windows.IO_STATUS_BLOCK
		size   int64
	)
	err = windows.NtCreateFile(
		&handle,
		access,
		attributes,
		&status,
		&size,
		windows.FILE_ATTRIBUTE_NORMAL,
		windowsShareAll,
		disposition,
		options|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT,
		0,
		0,
	)
	return handle, err
}

func validateWindowsContainedHandle(handle windows.Handle, name string, directory bool) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return fmt.Errorf("inspect contained target %q: %w", name, err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("refuse reparse-point contained target %q", name)
	}
	isDirectory := info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	if isDirectory != directory {
		return fmt.Errorf("refuse unexpected contained target type %q", name)
	}
	if !directory && info.NumberOfLinks > 1 {
		return fmt.Errorf("refuse hard-linked contained file %q", name)
	}
	return nil
}

func windowsContainedDestinationMode(parent windows.Handle, base string, requested os.FileMode, relative string) (os.FileMode, error) {
	handle, err := openWindowsRelative(parent, base, os.O_RDONLY, windows.FILE_NON_DIRECTORY_FILE)
	if err != nil {
		if isWindowsPathNotFound(err) {
			return requested.Perm(), nil
		}
		return 0, fmt.Errorf("inspect contained destination %q: %w", relative, err)
	}
	if err := validateWindowsContainedHandle(handle, relative, false); err != nil {
		_ = windows.CloseHandle(handle)
		return 0, err
	}
	file := os.NewFile(uintptr(handle), "")
	info, err := file.Stat()
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return 0, err
	}
	return info.Mode().Perm(), nil
}

func createWindowsContainedTemporary(parent windows.Handle, base string) (string, windows.Handle, error) {
	for range 100 {
		var suffix [12]byte
		if _, err := io.ReadFull(rand.Reader, suffix[:]); err != nil {
			return "", windows.InvalidHandle, fmt.Errorf("generate contained temporary name: %w", err)
		}
		name := "." + base + ".tmp-" + hex.EncodeToString(suffix[:])
		handle, err := openWindowsRelative(
			parent,
			name,
			os.O_RDWR|os.O_CREATE|os.O_EXCL,
			windows.FILE_NON_DIRECTORY_FILE,
		)
		if err == nil {
			return name, handle, nil
		}
		if !errors.Is(err, windows.ERROR_FILE_EXISTS) && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return "", windows.InvalidHandle, fmt.Errorf("create contained temporary file: %w", err)
		}
	}
	return "", windows.InvalidHandle, fmt.Errorf("create contained temporary file: exhausted name attempts")
}

func renameWindowsRelative(handle, parent windows.Handle, destination string) error {
	name, err := windows.UTF16FromString(destination)
	if err != nil {
		return err
	}
	nameBytes := len(name)*2 - 2
	var layout windowsFileRenameInformation
	buffer := make([]byte, int(unsafe.Offsetof(layout.FileName))+nameBytes)
	info := (*windowsFileRenameInformation)(unsafe.Pointer(&buffer[0]))
	info.ReplaceIfExists = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
	info.RootDirectory = parent
	info.FileNameLength = uint32(nameBytes)
	copy((*[windows.MAX_LONG_PATH]uint16)(unsafe.Pointer(&info.FileName[0]))[:nameBytes/2:nameBytes/2], name)
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(handle, &status, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation)
}

func removeWindowsRelative(parent windows.Handle, name string) error {
	handle, err := openWindowsRelative(parent, name, os.O_RDWR|os.O_EXCL, windows.FILE_NON_DIRECTORY_FILE)
	if err != nil {
		if isWindowsPathNotFound(err) {
			return nil
		}
		return err
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	flags := uint32(
		windows.FILE_DISPOSITION_DELETE |
			windows.FILE_DISPOSITION_POSIX_SEMANTICS |
			windows.FILE_DISPOSITION_IGNORE_READONLY_ATTRIBUTE,
	)
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(
		handle,
		&status,
		(*byte)(unsafe.Pointer(&flags)),
		uint32(unsafe.Sizeof(flags)),
		windows.FileDispositionInformationEx,
	)
}

func isWindowsPathNotFound(err error) bool {
	return errors.Is(err, windows.ERROR_FILE_NOT_FOUND) ||
		errors.Is(err, windows.ERROR_PATH_NOT_FOUND) ||
		errors.Is(err, windows.STATUS_NO_SUCH_FILE) ||
		errors.Is(err, windows.STATUS_OBJECT_NAME_NOT_FOUND) ||
		errors.Is(err, windows.STATUS_OBJECT_PATH_NOT_FOUND)
}
