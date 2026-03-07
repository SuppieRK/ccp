package runner

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

const (
	errRegisterPNPMFmt   = "register: %v"
	errUnexpectedPNPMFmt = "unexpected error: %v"
)

func mustPNPMRegistry(t *testing.T) *engine.ToolFilterRegistry {
	t.Helper()
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewPNPMFilter()); err != nil {
		t.Fatalf(errRegisterPNPMFmt, err)
	}
	return reg
}

func TestBuildExecPlanPNPMCases(t *testing.T) {
	reg := mustPNPMRegistry(t)
	cases := []struct {
		name         string
		args         []string
		expectedTool string
		dispatchKey  string
	}{
		{
			name:         "alias-resolves-pnpm",
			args:         []string{"pnpm.cmd", "list"},
			expectedTool: "pnpm",
			dispatchKey:  "pnpm|mode=list",
		},
		{
			name:         "unsupported-forces-neutral",
			args:         []string{"pnpm", "test"},
			expectedTool: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildExecPlan(tc.args, reg)
			if err != nil {
				t.Fatalf(errUnexpectedPNPMFmt, err)
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

func TestBuildExecPlanPNPMUnsafeInstallFallsBackToNeutral(t *testing.T) {
	reg := mustPNPMRegistry(t)
	plan, err := BuildExecPlan([]string{"pnpm", "install", "../evil"}, reg)
	if err != nil {
		t.Fatalf(errUnexpectedPNPMFmt, err)
	}
	if plan.Tool != "" {
		t.Fatalf("expected neutral fallback for unsafe install, got %q", plan.Tool)
	}
	if !plan.IsAmbiguous {
		t.Fatalf("expected plan ambiguity for unsafe install")
	}
}
