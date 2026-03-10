package runner

import (
	"strings"
	"testing"
)

func TestRunnerGitWorktreeListCompacts(t *testing.T) {
	skipWindowsGitFixture(t)
	r, script := newGitFixtureRunner(t, Options{}, []string{
		"#!/usr/bin/env sh",
		"echo '/tmp/repo  e77e6a2 [master]'",
		"echo '/tmp/wt-feature  e77e6a2 [feature]'",
		"exit 0",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "worktree", "list"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	for _, want := range []string{"/tmp/repo e77e6a2 [master]", "/tmp/wt-feature e77e6a2 [feature]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got %q", want, out)
		}
	}
}

func TestRunnerGitWorktreeVerbosePassthrough(t *testing.T) {
	skipWindowsGitFixture(t)
	r, script := newGitFixtureRunner(t, Options{}, []string{
		"#!/usr/bin/env sh",
		"echo '/tmp/repo  e77e6a2 [master]'",
		"echo '  locked: reason'",
		"exit 0",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "worktree", "list", "--verbose"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "locked: reason") {
		t.Fatalf("expected passthrough output, got %q", out)
	}
}
