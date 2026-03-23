//go:build !windows

package core

import (
	"os"
	"syscall"
)

func defaultExecutionSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
