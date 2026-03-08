package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/cli"
	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/runner"
)

func TestBuildRegistryStartsEmpty(t *testing.T) {
	registry := engine.NewToolFilterRegistry()
	if got := registry.Resolve("ls"); got != nil {
		t.Fatalf("expected empty registry before runtime wiring, got %q", got.Tool())
	}
}

func TestBuildRuntimeUsesSharedRegistryForPlannerAndEngine(t *testing.T) {
	opts := cli.Options{CommandArgs: []string{"ls"}}
	r, err := buildRuntime(opts)
	if err != nil {
		t.Fatalf("build runtime: %v", err)
	}
	registry := r.Registry()
	if registry == nil {
		t.Fatal("expected runner registry")
	}
	if registry.Resolve("ls") == nil {
		t.Fatal("expected ls filter in shared registry")
	}
	if registry.Resolve("gradlew") == nil {
		t.Fatal("expected gradle filter aliases in shared registry")
	}
	plan, err := runner.BuildExecPlan([]string{"ls"}, registry)
	if err != nil {
		t.Fatalf("build plan with shared registry: %v", err)
	}
	if plan.Tool != "" {
		t.Fatalf("expected default ls passthrough tool binding, got %q", plan.Tool)
	}
	longPlan, err := runner.BuildExecPlan([]string{"ls", "-l"}, registry)
	if err != nil {
		t.Fatalf("build long ls plan with shared registry: %v", err)
	}
	if longPlan.Tool != "ls" {
		t.Fatalf("expected ls tool for long listing shape, got %q", longPlan.Tool)
	}
}

func TestUsageTextIncludesHelpFlag(t *testing.T) {
	got := usageText()
	if got == "" {
		t.Fatal("expected non-empty usage text")
	}
	if !containsAll(got, []string{
		"ccp - command compression proxy for coding-agent workflows",
		"Usage:",
		"Execution flags:",
		"Lifecycle commands:",
		"Notes:",
		"--confidential",
		"init",
		"gain                  Show token savings summary and recent proof output",
		"Run ccp gain after install or init to verify savings on real work.",
		"--raw preserves native output unless --confidential is also used.",
	}) {
		t.Fatalf("usage text missing expected sections: %q", got)
	}
}

func TestMainWithoutExecutionCommandPrintsUsageAndExitsNonZero(t *testing.T) {
	if os.Getenv("CCP_MAIN_TEST_HELPER") == "1" {
		os.Args = []string{"ccp"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMainWithoutExecutionCommandPrintsUsageAndExitsNonZero")
	cmd.Env = append(os.Environ(), "CCP_MAIN_TEST_HELPER=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected non-zero exit, got err=%v", err)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit code, got %d", exitErr.ExitCode())
	}
	if !containsAll(stderr.String(), []string{"Usage:", "Execution flags:", "Lifecycle commands:"}) {
		t.Fatalf("expected usage on stderr, got %q", stderr.String())
	}
}

func TestMainLifecycleHelpPrintsStructuredHelpAndExitsZero(t *testing.T) {
	if os.Getenv("CCP_MAIN_LIFECYCLE_HELPER") == "1" {
		os.Args = []string{"ccp", "history", "--help"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMainLifecycleHelpPrintsStructuredHelpAndExitsZero")
	cmd.Env = append(os.Environ(), "CCP_MAIN_LIFECYCLE_HELPER=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("expected zero exit for lifecycle help, got %v", err)
	}
	if !containsAll(stderr.String(), []string{"ccp history - show recorded command history", "Usage:", "Flags:", "Notes:"}) {
		t.Fatalf("expected structured lifecycle help on stderr, got %q", stderr.String())
	}
}

func containsAll(s string, parts []string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
