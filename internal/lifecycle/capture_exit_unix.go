//go:build !windows

package lifecycle

import (
	"os/exec"
	"syscall"
)

func captureExitCode(exitErr *exec.ExitError) int {
	if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	return max(exitErr.ExitCode(), 1)
}
