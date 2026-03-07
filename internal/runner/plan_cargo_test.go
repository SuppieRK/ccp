package runner

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

func mustCargoRegistry(t *testing.T) *engine.ToolFilterRegistry {
	t.Helper()
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewCargoToolFilter()); err != nil {
		t.Fatalf("register cargo: %v", err)
	}
	return reg
}

func TestBuildExecPlanCargoDispatchesKnownSubcommands(t *testing.T) {
	reg := mustCargoRegistry(t)

	tests := []struct {
		args     []string
		dispatch string
	}{
		{args: []string{"cargo", "test", "./..."}, dispatch: "cargo test"},
		{args: []string{"cargo", "build"}, dispatch: "cargo build"},
		{args: []string{"cargo", "check"}, dispatch: "cargo check"},
		{args: []string{"cargo", "clippy"}, dispatch: "cargo clippy"},
	}
	for _, tc := range tests {
		plan, err := BuildExecPlan(tc.args, reg)
		if err != nil {
			t.Fatalf("unexpected error for %#v: %v", tc.args, err)
		}
		if plan.Tool != "cargo" || plan.DispatchKey != tc.dispatch {
			t.Fatalf("unexpected plan for %#v: %#v", tc.args, plan)
		}
	}
}

func TestBuildExecPlanCargoRunAmbiguousPassthrough(t *testing.T) {
	reg := mustCargoRegistry(t)
	plan, err := BuildExecPlan([]string{"cargo", "run", "--", "hello"}, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Tool != "" || !plan.IsAmbiguous {
		t.Fatalf("expected ambiguous passthrough, got %#v", plan)
	}
}
