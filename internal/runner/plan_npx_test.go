package runner

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

const (
	errRegisterNPXFmt   = "register: %v"
	errUnexpectedNPXFmt = "unexpected error: %v"
)

func TestBuildExecPlanNPXCases(t *testing.T) {
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewNPXFilter()); err != nil {
		t.Fatalf(errRegisterNPXFmt, err)
	}

	cases := []struct {
		name         string
		args         []string
		expectedTool string
		dispatchKey  string
	}{
		{
			name:         "route-dispatch",
			args:         []string{"npx", "tsc", "--noEmit"},
			expectedTool: "npx",
			dispatchKey:  "npx tsc",
		},
		{
			name:         "unsupported-passthrough",
			args:         []string{"npx", "unknown-tool"},
			expectedTool: "",
		},
		{
			name:         "package-flag-passthrough",
			args:         []string{"npx", "-p", "cowsay", "lolcat"},
			expectedTool: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildExecPlan(tc.args, reg, false)
			if err != nil {
				t.Fatalf(errUnexpectedNPXFmt, err)
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
