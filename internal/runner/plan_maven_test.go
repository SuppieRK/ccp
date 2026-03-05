package runner

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

func TestBuildExecPlanMavenWrapperAliasResolvesMavenTool(t *testing.T) {
	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewMavenFilter()); err != nil {
		t.Fatalf("register: %v", err)
	}
	plan, err := BuildExecPlan([]string{"./mvnw", "test"}, registry, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Tool != "maven" {
		t.Fatalf("expected maven tool, got %q", plan.Tool)
	}
	if plan.Name != "./mvnw" {
		t.Fatalf("expected direct wrapper execution, got %q", plan.Name)
	}
	if plan.DispatchKey != "maven|parallel=0" {
		t.Fatalf("expected non-parallel dispatch key, got %q", plan.DispatchKey)
	}
}
