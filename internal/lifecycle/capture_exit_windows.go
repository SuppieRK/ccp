//go:build windows

package lifecycle

import "os/exec"

func captureExitCode(exitErr *exec.ExitError) int {
	return max(exitErr.ExitCode(), 1)
}
