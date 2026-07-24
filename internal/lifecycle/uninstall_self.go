package lifecycle

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func scheduleExecutableRemoval(exePath string) error {
	exePath = filepath.Clean(exePath)
	if exePath == "" || exePath == "." {
		return fmt.Errorf("resolve executable path for uninstall")
	}

	scriptPath, body, err := uninstallRemovalScript(exePath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(scriptPath, []byte(body), uninstallRemovalScriptMode()); err != nil {
		return err
	}

	cmd := uninstallRemovalCommand(scriptPath)
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { _ = devNull.Close() }()
	cmd.Stdin = devNull
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	return cmd.Start()
}

func uninstallRemovalScript(exePath string) (string, string, error) {
	name := fmt.Sprintf("cmdshape-uninstall-%d", time.Now().UnixNano())
	if runtime.GOOS == "windows" {
		scriptPath := filepath.Join(os.TempDir(), name+".cmd")
		body := "@echo off\r\n" +
			"ping 127.0.0.1 -n 3 >NUL\r\n" +
			"del /f /q " + windowsQuoteArg(exePath) + "\r\n" +
			"del /f /q \"%~f0\"\r\n"
		return scriptPath, body, nil
	}

	scriptPath := filepath.Join(os.TempDir(), name+".sh")
	body := "#!/bin/sh\n" +
		"sleep 1\n" +
		"rm -f -- " + shellQuoteArg(exePath) + "\n" +
		"rm -f -- \"$0\"\n"
	return scriptPath, body, nil
}

func uninstallRemovalScriptMode() os.FileMode {
	if runtime.GOOS == "windows" {
		return 0o644
	}
	return 0o700
}

func uninstallRemovalCommand(scriptPath string) *exec.Cmd {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command(windowsCmdPath(), "/c", scriptPath)
	} else {
		cmd = exec.Command("/bin/sh", scriptPath)
	}
	cmd.Env = uninstallRemovalEnv()
	return cmd
}

func uninstallRemovalEnv() []string {
	if runtime.GOOS == "windows" {
		root := windowsSystemRoot()
		return []string{
			"PATH=" + filepath.Join(root, "System32"),
			"SystemRoot=" + root,
			"WINDIR=" + root,
		}
	}
	return []string{"PATH=/usr/bin:/bin"}
}

func windowsCmdPath() string {
	return filepath.Join(windowsSystemRoot(), "System32", "cmd.exe")
}

func windowsSystemRoot() string {
	return filepath.Clean(`C:\Windows`)
}

func shellQuoteArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func windowsQuoteArg(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
