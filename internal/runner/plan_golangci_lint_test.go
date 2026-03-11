package runner

import (
	"slices"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

func TestBuildExecPlanGolangciLintCases(t *testing.T) {
	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewGolangciLintFilter()); err != nil {
		t.Fatalf("register golangci-lint: %v", err)
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
			args:         []string{"golangci-lint", "run", "./..."},
			expectedTool: "golangci-lint",
			dispatchKey:  "golangci-lint",
			expectedArgs: []string{"run", "--out-format", "json", "./..."},
		},
		{
			name:         "script-path-alias-dispatch",
			args:         []string{"./golangci-lint", "./..."},
			expectedTool: "golangci-lint",
			dispatchKey:  "golangci-lint",
			expectedArgs: []string{"run", "--out-format", "json", "./..."},
		},
		{
			name:         "machine-readable-passthrough",
			args:         []string{"golangci-lint", "run", "--out-format", "json", "./..."},
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
