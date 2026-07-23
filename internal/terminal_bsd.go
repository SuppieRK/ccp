//go:build darwin || freebsd || netbsd || openbsd

package core

import (
	"os"

	"golang.org/x/sys/unix"
)

func fileIsTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	_, err := unix.IoctlGetTermios(int(file.Fd()), unix.TIOCGETA)
	return err == nil
}
