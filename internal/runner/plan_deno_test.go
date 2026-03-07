package runner

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

const (
	errRegisterDenoFilterFmt = "register deno filter: %v"
	errUnexpectedDenoFmt     = "unexpected error: %v"
)

func mustDenoRegistry(t *testing.T) *engine.ToolFilterRegistry {
	t.Helper()
	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewDenoFilter()); err != nil {
		t.Fatalf(errRegisterDenoFilterFmt, err)
	}
	return registry
}

func TestBuildExecPlanDenoModes(t *testing.T) {
	registry := mustDenoRegistry(t)

	plan, err := BuildExecPlan([]string{"deno", "run", "main.ts"}, registry)
	if err != nil {
		t.Fatalf(errUnexpectedDenoFmt, err)
	}
	if plan.Tool != "deno" {
		t.Fatalf("expected deno tool binding, got %q", plan.Tool)
	}
	if plan.DispatchKey != "deno|mode=run" {
		t.Fatalf("unexpected dispatch key: %q", plan.DispatchKey)
	}
}

func TestBuildExecPlanDenoPassthroughCases(t *testing.T) {
	registry := mustDenoRegistry(t)
	cases := []struct {
		name string
		args []string
	}{
		{name: "repl", args: []string{"deno", "repl"}},
		{name: "structured-output", args: []string{"deno", "test", "--json"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildExecPlan(tc.args, registry)
			if err != nil {
				t.Fatalf(errUnexpectedDenoFmt, err)
			}
			if plan.Tool != "" {
				t.Fatalf("expected passthrough tool binding, got %q", plan.Tool)
			}
		})
	}
}
