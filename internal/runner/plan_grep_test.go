package runner

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

const (
	errRegisterGrepFmt   = "register grep filter: %v"
	errUnexpectedGrepFmt = "unexpected error: %v"
)

func mustGrepRegistry(t *testing.T) *engine.ToolFilterRegistry {
	t.Helper()
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewGrepFilter()); err != nil {
		t.Fatalf(errRegisterGrepFmt, err)
	}
	return reg
}

func TestBuildExecPlanGrepUsesPreferredSubstitutionWhenAvailable(t *testing.T) {
	restore := stubLookPath(func(file string) (string, error) {
		if file == "rg" {
			return "/usr/bin/rg", nil
		}
		return "", errors.New("unexpected lookup: " + file)
	})
	defer restore()

	reg := mustGrepRegistry(t)

	plan, err := BuildExecPlan([]string{"grep", "needle"}, reg, false)
	if err != nil {
		t.Fatalf(errUnexpectedGrepFmt, err)
	}
	if plan.Tool != "grep" {
		t.Fatalf("expected grep tool, got %q", plan.Tool)
	}
	if plan.Name != "rg" {
		t.Fatalf("expected preferred substitution rg, got %q", plan.Name)
	}
	if plan.FallbackName != "grep" {
		t.Fatalf("expected grep fallback, got %q", plan.FallbackName)
	}
	if len(plan.Args) == 0 || plan.Args[len(plan.Args)-1] != "." {
		t.Fatalf("expected default path '.', got %#v", plan.Args)
	}
	if !containsArg(plan.Args, "--color=never") {
		t.Fatalf("expected rg normalization flags, got %#v", plan.Args)
	}
	if !strings.Contains(plan.DispatchKey, "max=50") {
		t.Fatalf("expected dispatch max metadata, got %q", plan.DispatchKey)
	}
}

func TestBuildExecPlanGrepFallsBackToNativeWhenPreferredUnavailable(t *testing.T) {
	restore := stubLookPath(func(file string) (string, error) {
		return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
	})
	defer restore()

	reg := mustGrepRegistry(t)

	plan, err := BuildExecPlan([]string{"grep", "needle"}, reg, false)
	if err != nil {
		t.Fatalf(errUnexpectedGrepFmt, err)
	}
	if plan.Name != "grep" {
		t.Fatalf("expected native grep fallback, got %q", plan.Name)
	}
	if !containsArg(plan.Args, "-r") || !containsArg(plan.Args, "-H") || !containsArg(plan.Args, "-n") {
		t.Fatalf("expected recursive fallback flags, got %#v", plan.Args)
	}
	if plan.FallbackName != "" {
		t.Fatalf("expected no second-level fallback, got %q", plan.FallbackName)
	}
}

func TestBuildExecPlanGrepUnsafeRegexAmbiguous(t *testing.T) {
	restore := stubLookPath(func(file string) (string, error) { return "/usr/bin/" + file, nil })
	defer restore()

	reg := mustGrepRegistry(t)

	plan, err := BuildExecPlan([]string{"grep", `a\+b`, "."}, reg, false)
	if err != nil {
		t.Fatalf(errUnexpectedGrepFmt, err)
	}
	if !plan.IsAmbiguous {
		t.Fatal("expected ambiguous plan")
	}
	if plan.Tool != "" {
		t.Fatalf("expected neutral passthrough for ambiguous regex, got %q", plan.Tool)
	}
	if _, err := BuildExecPlan([]string{"grep", `a\+b`, "."}, reg, true); err == nil {
		t.Fatal("expected strict mode to reject unsafe regex translation")
	}
}

func TestBuildExecPlanGrepStrictAddsNoMatchSuppressionMarker(t *testing.T) {
	restore := stubLookPath(func(file string) (string, error) { return "", &exec.Error{Name: file, Err: exec.ErrNotFound} })
	defer restore()

	reg := mustGrepRegistry(t)

	plan, err := BuildExecPlan([]string{"grep", "needle", "."}, reg, true)
	if err != nil {
		t.Fatalf(errUnexpectedGrepFmt, err)
	}
	if !strings.Contains(plan.DispatchKey, "strict_no_match=1") {
		t.Fatalf("expected strict dispatch marker, got %q", plan.DispatchKey)
	}
}

func stubLookPath(fn func(file string) (string, error)) func() {
	oldFn := lookPathFn
	oldCache := lookPathOK
	lookPathFn = fn
	lookPathOK = map[string]bool{}
	return func() {
		lookPathFn = oldFn
		lookPathOK = oldCache
	}
}

func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}
