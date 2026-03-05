package runner

import (
	"slices"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

const (
	errRegisterNodeFilterFmt = "register node filter: %v"
	errUnexpectedNodeFmt     = "unexpected error: %v"
	nodeScriptName           = "app.js"
)

func TestBuildExecPlanNodeCases(t *testing.T) {
	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewNodeFilter()); err != nil {
		t.Fatalf(errRegisterNodeFilterFmt, err)
	}

	cases := []struct {
		name         string
		args         []string
		expectedTool string
		dispatchKey  string
		expectedArgs []string
	}{
		{
			name:         "script-filtered",
			args:         []string{"node", nodeScriptName},
			expectedTool: "node",
			dispatchKey:  "node|mode=runtime",
			expectedArgs: []string{nodeScriptName},
		},
		{
			name:         "interactive-passthrough",
			args:         []string{"node", "-i"},
			expectedTool: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildExecPlan(tc.args, registry, false)
			if err != nil {
				t.Fatalf(errUnexpectedNodeFmt, err)
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

func TestBuildExecPlanNPXNodeDelegates(t *testing.T) {
	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewNPXFilter()); err != nil {
		t.Fatalf("register npx filter: %v", err)
	}
	if err := registry.Register(filters.NewNodeFilter()); err != nil {
		t.Fatalf(errRegisterNodeFilterFmt, err)
	}

	plan, err := BuildExecPlan([]string{"npx", "node", nodeScriptName}, registry, false)
	if err != nil {
		t.Fatalf(errUnexpectedNodeFmt, err)
	}
	if plan.Tool != "npx" {
		t.Fatalf("expected npx parent tool binding, got %q", plan.Tool)
	}
	if plan.DispatchKey != "npx node" {
		t.Fatalf("expected npx node dispatch key, got %q", plan.DispatchKey)
	}
}
