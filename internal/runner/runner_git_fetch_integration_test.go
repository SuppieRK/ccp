package runner

import (
	"strings"
	"testing"
)

func TestRunnerGitFetchCompactsNonEmptySuccess(t *testing.T) {
	skipWindowsGitFixture(t)
	r, script := newGitFixtureRunner(t, Options{}, []string{
		"#!/usr/bin/env sh",
		"echo 'From /tmp/remote' 1>&2",
		"echo ' * [new branch]      feature-x  -> origin/feature-x' 1>&2",
		"echo '   c4b832f..6fe960e  main       -> origin/main' 1>&2",
		"echo ' * [new tag]         v2         -> v2' 1>&2",
		"exit 0",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "fetch", "origin"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	for _, want := range []string{"git fetch: ok 3 ref updates", "1 new branch", "1 new tag"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got %q", want, out)
		}
	}
}

func TestRunnerGitFetchEmptySuccessStaysEmpty(t *testing.T) {
	skipWindowsGitFixture(t)
	r, script := newGitFixtureRunner(t, Options{}, []string{
		"#!/usr/bin/env sh",
		"exit 0",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "fetch", "origin"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("expected empty output, got %q", out)
	}
}

func TestRunnerGitFetchDetailModePassthroughAndExitParity(t *testing.T) {
	skipWindowsGitFixture(t)
	r, script := newGitFixtureRunner(t, Options{}, []string{
		"#!/usr/bin/env sh",
		"echo 'From /tmp/remote' 1>&2",
		"echo ' * [new branch]      feature-x  -> origin/feature-x' 1>&2",
		"exit 7",
	})

	out, code := captureCombined(t, func() int {
		return r.Run([]string{script, "fetch", "--verbose", "origin"})
	})
	if code != 7 {
		t.Fatalf("expected exit code 7, got %d", code)
	}
	for _, want := range []string{"From /tmp/remote", "feature-x  -> origin/feature-x"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in passthrough output, got %q", want, out)
		}
	}
}
