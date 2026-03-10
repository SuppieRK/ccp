package runner

import (
	"slices"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

func TestBuildExecPlanPrettierCases(t *testing.T) {
	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewPrettierFilter()); err != nil {
		t.Fatalf("register prettier filter: %v", err)
	}

	cases := []struct {
		name         string
		args         []string
		expectedTool string
		dispatchKey  string
		expectedArgs []string
	}{
		{
			name:         "route-check",
			args:         []string{"prettier", "--check", "src/good.js"},
			expectedTool: "prettier",
			dispatchKey:  "prettier|mode=check",
			expectedArgs: []string{"--check", "src/good.js"},
		},
		{
			name:         "unsupported-shape-passthrough",
			args:         []string{"prettier", "--check", "--ignore-unknown", "src/good.js"},
			expectedTool: "",
		},
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
