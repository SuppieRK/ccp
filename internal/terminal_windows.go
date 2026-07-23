//go:build windows

package core

import (
	"os"

	"golang.org/x/sys/windows"
)

func fileIsTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(file.Fd()), &mode) == nil
}
