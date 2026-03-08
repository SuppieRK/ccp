package lifecycle

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunInitHelpOutput(t *testing.T) {
	out := captureLifecycleStderr(t, func() error {
		return RunInit([]string{"--help"})
	})
	assertLifecycleHelpContains(t, out, []string{
		"ccp init - install or update supported agent integrations",
		"Usage:",
		"Flags:",
		"Notes:",
		"--tools",
		"~/.config/ccp/init.json",
	})
}

func TestRunGainHelpOutput(t *testing.T) {
	out := captureLifecycleStderr(t, func() error {
		return RunGain([]string{"--help"}, "")
	})
	assertLifecycleHelpContains(t, out, []string{
		"ccp gain - show token savings history",
		"Usage:",
		"Flags:",
		"Notes:",
		"--period",
		"--format",
		"--table",
		"Run ccp gain after install or init to verify savings on real work.",
	})
}

func TestRunHistoryHelpOutput(t *testing.T) {
	out := captureLifecycleStderr(t, func() error {
		return RunHistory([]string{"--help"}, "")
	})
	assertLifecycleHelpContains(t, out, []string{
		"ccp history - show recorded command history",
		"Usage:",
		"Flags:",
		"Notes:",
		"--since",
		"--tool",
	})
}

func TestRunUpgradeHelpOutput(t *testing.T) {
	out := captureLifecycleStderr(t, func() error {
		return RunUpgrade([]string{"--help"})
	})
	assertLifecycleHelpContains(t, out, []string{
		"ccp upgrade - upgrade ccp from GitHub Releases",
		"Usage:",
		"Flags:",
		"Notes:",
		"--version",
	})
}

func TestRunUninstallHelpOutput(t *testing.T) {
	out := captureLifecycleStderr(t, func() error {
		return RunUninstall([]string{"--help"})
	})
	assertLifecycleHelpContains(t, out, []string{
		"ccp uninstall - remove supported agent integrations",
		"Usage:",
		"Flags:",
		"Notes:",
		"--tools",
		"~/.config/ccp/init.json",
	})
}

func captureLifecycleStderr(t *testing.T, fn func() error) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() {
		os.Stderr = orig
	}()

	runErr := fn()
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	if runErr != nil {
		t.Fatalf("run function: %v", runErr)
	}
	return buf.String()
}

func assertLifecycleHelpContains(t *testing.T, out string, parts []string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(out, part) {
			t.Fatalf("expected help output to contain %q, got %q", part, out)
		}
	}
}
