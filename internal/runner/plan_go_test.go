package runner

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

func mustGoRegistry(t *testing.T) *engine.ToolFilterRegistry {
	t.Helper()
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewGoToolFilter()); err != nil {
		t.Fatalf("register go: %v", err)
	}
	return reg
}

func TestBuildExecPlanGoDispatchesKnownSubcommands(t *testing.T) {
	reg := mustGoRegistry(t)

	tests := []struct {
		args     []string
		dispatch string
	}{
		{args: []string{"go", "test", "./..."}, dispatch: "go test"},
		{args: []string{"go", "build", "./..."}, dispatch: "go build"},
	}
	for _, tc := range tests {
		plan, err := BuildExecPlan(tc.args, reg, false)
		if err != nil {
			t.Fatalf("unexpected error for %#v: %v", tc.args, err)
		}
		if plan.Tool != "go" || plan.DispatchKey != tc.dispatch {
			t.Fatalf("unexpected plan for %#v: %#v", tc.args, plan)
		}
	}
}

func TestBuildExecPlanGoUnsupportedPassthrough(t *testing.T) {
	reg := mustGoRegistry(t)
	plan, err := BuildExecPlan([]string{"go", "run", "."}, reg, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Tool != "" {
		t.Fatalf("expected passthrough for unsupported subcommand, got %#v", plan)
	}
}
