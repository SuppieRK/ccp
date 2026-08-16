//go:build !windows

package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

var managedSignalGracePeriod = 2 * time.Second

func configureManagedCommand(cmd *exec.Cmd, ctx context.Context) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		sig := syscall.SIGKILL
		if cause, ok := errors.AsType[executionSignal](context.Cause(ctx)); ok {
			if forwarded, ok := cause.signal.(syscall.Signal); ok {
				sig = forwarded
			}
		}
		processGroup := -cmd.Process.Pid
		err := syscall.Kill(processGroup, sig)
		if err == nil || errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			if err == nil && sig != syscall.SIGKILL {
				deadline := time.Now().Add(managedSignalGracePeriod)
				for time.Now().Before(deadline) {
					if probeErr := syscall.Kill(processGroup, 0); errors.Is(probeErr, syscall.ESRCH) {
						return nil
					}
					time.Sleep(10 * time.Millisecond)
				}
				if killErr := syscall.Kill(processGroup, syscall.SIGKILL); killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
					return killErr
				}
			}
			return nil
		}
		return err
	}
}

func nativeExitCode(exitErr *exec.ExitError) (int, bool) {
	if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal()), true
	}
	code := exitErr.ExitCode()
	return code, code >= 0
}

func isHardKillExitCode(code int) bool { return code == 128+int(syscall.SIGKILL) }
