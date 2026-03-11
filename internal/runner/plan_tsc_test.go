package runner

import (
	"slices"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

func TestBuildExecPlanTscCases(t *testing.T) {
	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewTscFilter()); err != nil {
		t.Fatalf("register tsc filter: %v", err)
	}

	cases := []struct {
		name         string
		args         []string
		expectedTool string
		dispatchKey  string
		expectedArgs []string
	}{
		{
			name:         "direct-dispatch",
			args:         []string{"tsc", "--noEmit", "-p", "tsconfig.json"},
			expectedTool: "tsc",
			dispatchKey:  "tsc",
			expectedArgs: []string{"--noEmit", "-p", "tsconfig.json", "--pretty", "false"},
		},
		{
			name:         "pretty-enabled-passthrough",
			args:         []string{"tsc", "--pretty", "true", "--noEmit"},
			expectedTool: "",
		},
		{
			name:         "watch-passthrough",
			args:         []string{"tsc", "--watch"},
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
