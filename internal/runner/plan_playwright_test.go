package runner

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

func TestBuildExecPlanPlaywrightCases(t *testing.T) {
	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewPlaywrightFilter()); err != nil {
		t.Fatalf("register playwright: %v", err)
	}

	for _, tc := range []struct {
		name         string
		args         []string
		expectedTool string
		dispatchKey  string
	}{
		{name: "direct dispatch", args: []string{"playwright", "test"}, expectedTool: "playwright", dispatchKey: "playwright"},
		{name: "alias dispatch", args: []string{"./playwright", "test", "--grep", "auth"}, expectedTool: "playwright", dispatchKey: "playwright"},
		{name: "reporter passthrough", args: []string{"playwright", "test", "--reporter=json"}},
		{name: "subcommand passthrough", args: []string{"playwright", "show-report"}},
	} {
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
		})
	}
}
