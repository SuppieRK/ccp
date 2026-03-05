package runner

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

func mustFindRegistry(t *testing.T) *engine.ToolFilterRegistry {
	t.Helper()
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewFindFilter()); err != nil {
		t.Fatalf("register find: %v", err)
	}
	return reg
}

func TestBuildExecPlanFindDefaultsToSystemFind(t *testing.T) {
	reg := mustFindRegistry(t)

	plan, err := BuildExecPlan([]string{"find", ".", "-name", "*.go"}, reg, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Name != "find" && plan.Name != "fd" {
		t.Fatalf("expected find/fd execution name, got %q", plan.Name)
	}
	if plan.Name == "fd" && plan.FallbackName != "find" {
		t.Fatalf("expected fallback find when using fd substitution, got %q", plan.FallbackName)
	}
	if !strings.Contains(plan.DispatchKey, "pattern=*.go") {
		t.Fatalf("expected dispatch to include pattern, got %q", plan.DispatchKey)
	}
}

func TestBuildExecPlanFindAvoidsUnsafeSubstitution(t *testing.T) {
	reg := mustFindRegistry(t)

	plan, err := BuildExecPlan([]string{"find", ".", "-delete"}, reg, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Name != "find" {
		t.Fatalf("expected system find for unsafe shape, got %q", plan.Name)
	}
	if plan.FallbackName != "" {
		t.Fatalf("expected no fallback binding when no substitution, got %q", plan.FallbackName)
	}
}
