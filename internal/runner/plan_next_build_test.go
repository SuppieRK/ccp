package runner

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

func TestBuildExecPlanNextBuildCases(t *testing.T) {
	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewNextBuildFilter()); err != nil {
		t.Fatalf("register next build filter: %v", err)
	}

	for _, tc := range []struct {
		name         string
		args         []string
		expectedTool string
		dispatchKey  string
	}{
		{name: "direct dispatch", args: []string{"next", "build"}, expectedTool: "next", dispatchKey: "next-build"},
		{name: "path alias dispatch", args: []string{"./next", "build"}, expectedTool: "next", dispatchKey: "next-build"},
		{name: "debug passthrough", args: []string{"next", "build", "--debug"}},
		{name: "dev passthrough", args: []string{"next", "dev"}},
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
