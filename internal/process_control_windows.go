//go:build windows

package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

func configureManagedCommand(cmd *exec.Cmd, _ context.Context) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		taskkill := exec.Command(taskkillExecutablePath(), "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		taskkillErr := taskkill.Run()
		if taskkillErr == nil {
			return nil
		}
		if killErr := cmd.Process.Kill(); killErr == nil || errors.Is(killErr, os.ErrProcessDone) {
			return nil
		}
		return taskkillErr
	}
}

func taskkillExecutablePath() string {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = os.Getenv("WINDIR")
	}
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	return filepath.Join(systemRoot, "System32", "taskkill.exe")
}

func nativeExitCode(exitErr *exec.ExitError) (int, bool) {
	code := exitErr.ExitCode()
	return code, code >= 0
}

func isHardKillExitCode(int) bool { return false }
