package runner

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

func TestBuildExecPlanMypyCases(t *testing.T) {
	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewMypyFilter()); err != nil {
		t.Fatalf("register mypy: %v", err)
	}

	cases := []struct {
		name         string
		args         []string
		expectedTool string
		dispatchKey  string
	}{
		{
			name:         "direct-dispatch",
			args:         []string{"mypy", "src"},
			expectedTool: "mypy",
			dispatchKey:  "mypy",
		},
		{
			name:         "alias-dispatch",
			args:         []string{"./mypy", "src"},
			expectedTool: "mypy",
			dispatchKey:  "mypy",
		},
		{
			name:         "machine-readable-passthrough",
			args:         []string{"mypy", "--output=json", "src"},
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
				t.Fatalf("unexpected tool: got %q want %q", plan.Tool, tc.expectedTool)
			}
			if tc.dispatchKey != "" && plan.DispatchKey != tc.dispatchKey {
				t.Fatalf("unexpected dispatch key: got %q want %q", plan.DispatchKey, tc.dispatchKey)
			}
		})
	}
}
