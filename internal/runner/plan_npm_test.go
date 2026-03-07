package runner

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

func TestBuildExecPlanNPMCases(t *testing.T) {
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewNPMFilter()); err != nil {
		t.Fatalf("register: %v", err)
	}

	cases := []struct {
		name         string
		args         []string
		expectedTool string
		dispatchKey  string
	}{
		{
			name:         "alias-run-resolves-npm",
			args:         []string{"npm.cmd", "run", "build"},
			expectedTool: "npm",
			dispatchKey:  "npm|mode=run",
		},
		{
			name:         "non-run-neutral-tool",
			args:         []string{"npm", "install"},
			expectedTool: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildExecPlan(tc.args, reg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if plan.Tool != tc.expectedTool {
				t.Fatalf("unexpected tool: got %q want %q", plan.Tool, tc.expectedTool)
			}
			if tc.dispatchKey != "" && plan.DispatchKey != tc.dispatchKey {
				t.Fatalf("unexpected dispatch key: got %q want %q", plan.DispatchKey, tc.dispatchKey)
			}
		})
	}
}
