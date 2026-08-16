//go:build !windows

package core

import (
	"os"
	"syscall"
)

func defaultExecutionSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

func executionSignalExitCode(signal os.Signal) int {
	if native, ok := signal.(syscall.Signal); ok {
		return 128 + int(native)
	}
	return 1
}
