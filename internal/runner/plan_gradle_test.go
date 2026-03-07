package runner

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

func TestBuildExecPlanGradleWrapperAliasResolvesGradleTool(t *testing.T) {
	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewGradleFilter()); err != nil {
		t.Fatalf("register gradle: %v", err)
	}
	plan, err := BuildExecPlan([]string{"./gradlew", "build"}, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Tool != "gradle" {
		t.Fatalf("expected gradle tool, got %q", plan.Tool)
	}
	if plan.Name != "./gradlew" {
		t.Fatalf("expected direct wrapper execution, got %q", plan.Name)
	}
}
