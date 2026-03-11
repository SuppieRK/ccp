package runner

import (
	"slices"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

func TestBuildExecPlanRuffCases(t *testing.T) {
	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewRuffFilter()); err != nil {
		t.Fatalf("register ruff: %v", err)
	}

	cases := []struct {
		name         string
		args         []string
		expectedTool string
		dispatchKey  string
		expectedArgs []string
	}{
		{name: "direct-dispatch", args: []string{"ruff", "src"}, expectedTool: "ruff", dispatchKey: "ruff", expectedArgs: []string{"check", "--output-format", "json", "src"}},
		{name: "check-dispatch", args: []string{"ruff", "check", "src"}, expectedTool: "ruff", dispatchKey: "ruff", expectedArgs: []string{"check", "--output-format", "json", "src"}},
		{name: "machine-readable-passthrough", args: []string{"ruff", "check", "--output-format", "json", "src"}, expectedTool: ""},
		{name: "format-passthrough", args: []string{"ruff", "format", "src"}, expectedTool: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildExecPlan(tc.args, registry)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if plan.Tool != tc.expectedTool {
				t.Fatalf("unexpected tool binding: got %q want %q", plan.Tool, tc.expectedTool)
			}
			if tc.dispatchKey != "" && plan.DispatchKey != tc.dispatchKey {
				t.Fatalf("unexpected dispatch key: got %q want %q", plan.DispatchKey, tc.dispatchKey)
			}
			if tc.expectedArgs != nil && !slices.Equal(plan.Args, tc.expectedArgs) {
				t.Fatalf("unexpected args: %#v", plan.Args)
			}
		})
	}
}
