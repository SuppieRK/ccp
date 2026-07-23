//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !windows

package core

import "os"

func fileIsTerminal(*os.File) bool {
	return false
}
