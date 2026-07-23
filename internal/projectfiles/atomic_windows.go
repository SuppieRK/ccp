//go:build windows

package projectfiles

import (
	"errors"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

const windowsReplaceAttempts = 25

var windowsReplaceMu sync.Mutex

func replaceFile(src, dst string) error {
	srcPath, err := windows.UTF16PtrFromString(src)
	if err != nil {
		return err
	}
	dstPath, err := windows.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}
	windowsReplaceMu.Lock()
	defer windowsReplaceMu.Unlock()

	for attempt := range windowsReplaceAttempts {
		err = windows.MoveFileEx(
			srcPath,
			dstPath,
			windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
		)
		if err == nil || !retryableWindowsReplaceError(err) {
			return err
		}
		if attempt+1 < windowsReplaceAttempts {
			time.Sleep(time.Duration(attempt+1) * time.Millisecond)
		}
	}
	return err
}

func retryableWindowsReplaceError(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}

func syncDirectory(string) error {
	return nil
}
