package runner

import (
	"slices"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

const (
	errRegisterPythonFmt   = "register: %v"
	errRegisterPytestFmt   = "register pytest: %v"
	errUnexpectedPythonFmt = "unexpected error: %v"
	pythonScriptName       = "script.py"
)

func mustPythonRegistry(t *testing.T) *engine.ToolFilterRegistry {
	t.Helper()
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewPythonFilter()); err != nil {
		t.Fatalf(errRegisterPythonFmt, err)
	}
	return reg
}

func mustPythonPytestRegistry(t *testing.T) *engine.ToolFilterRegistry {
	t.Helper()
	reg := mustPythonRegistry(t)
	if err := reg.Register(filters.NewPytestFilter()); err != nil {
		t.Fatalf(errRegisterPytestFmt, err)
	}
	return reg
}

func TestBuildExecPlanPythonAliasesResolveAndForceNeutral(t *testing.T) {
	reg := mustPythonRegistry(t)

	for _, bin := range []string{"python", "python3"} {
		plan, err := BuildExecPlan([]string{bin, "app.py"}, reg, false)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", bin, err)
		}
		if plan.Tool != "" {
			t.Fatalf("expected neutral passthrough tool for %s, got %q", bin, plan.Tool)
		}
		if plan.Name != bin {
			t.Fatalf("expected original binary preserved for %s, got %q", bin, plan.Name)
		}
		if !slices.Equal(plan.Args, []string{"app.py"}) {
			t.Fatalf("unexpected args for %s: %#v", bin, plan.Args)
		}
	}
}

func TestBuildExecPlanPythonNoArgAndIInteractiveAreAmbiguous(t *testing.T) {
	reg := mustPythonRegistry(t)

	cases := [][]string{{"python"}, {"python", "-i"}, {"python", "--interactive"}}
	for _, args := range cases {
		plan, err := BuildExecPlan(args, reg, false)
		if err != nil {
			t.Fatalf("unexpected error for %#v: %v", args, err)
		}
		if !plan.IsAmbiguous {
			t.Fatalf("expected ambiguous plan for %#v, got %#v", args, plan)
		}
		if plan.Tool != "" {
			t.Fatalf("expected neutral tool for ambiguous python invocation %#v, got %q", args, plan.Tool)
		}
	}
}

func TestBuildExecPlanPythonModuleInvocationCases(t *testing.T) {
	reg := mustPythonPytestRegistry(t)

	cases := []struct {
		name         string
		args         []string
		expectedTool string
		dispatchKey  string
		expectedArgs []string
	}{
		{
			name:         "pytest-module-routes-through-python",
			args:         []string{"python", "-m", "pytest", "-q"},
			expectedTool: "python",
			dispatchKey:  "pytest",
			expectedArgs: []string{"-m", "pytest", "-q", "--tb=short", "--no-header"},
		},
		{
			name:         "non-pytest-module-stays-passthrough",
			args:         []string{"python3", "-m", "pip", "list"},
			expectedTool: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildExecPlan(tc.args, reg, false)
			if err != nil {
				t.Fatalf(errUnexpectedPythonFmt, err)
			}
			if plan.Tool != tc.expectedTool {
				t.Fatalf("unexpected tool: got %q want %q", plan.Tool, tc.expectedTool)
			}
			if plan.DispatchKey != tc.dispatchKey {
				t.Fatalf("unexpected dispatch key: got %q want %q", plan.DispatchKey, tc.dispatchKey)
			}
			if tc.expectedArgs != nil && !slices.Equal(plan.Args, tc.expectedArgs) {
				t.Fatalf("unexpected args: %#v", plan.Args)
			}
		})
	}
}

func TestBuildExecPlanDirectPytestRouting(t *testing.T) {
	reg := mustPythonPytestRegistry(t)

	plan, err := BuildExecPlan([]string{"pytest", "-q"}, reg, false)
	if err != nil {
		t.Fatalf(errUnexpectedPythonFmt, err)
	}
	if plan.Tool != "pytest" {
		t.Fatalf("expected pytest tool, got %q", plan.Tool)
	}
	if plan.IsAmbiguous {
		t.Fatalf("did not expect pytest plan ambiguity: %#v", plan)
	}
	if !slices.Equal(plan.Args, []string{"-q", "--tb=short", "--no-header"}) {
		t.Fatalf("unexpected args: %#v", plan.Args)
	}
}

func TestBuildExecPlanPythonDoesNotInjectUnbufferedFlag(t *testing.T) {
	reg := mustPythonRegistry(t)

	args := []string{"python", pythonScriptName}
	plan, err := BuildExecPlan(args, reg, false)
	if err != nil {
		t.Fatalf(errUnexpectedPythonFmt, err)
	}
	if !slices.Equal(plan.Args, []string{pythonScriptName}) {
		t.Fatalf("expected args preserved without rewrite, got %#v", plan.Args)
	}
}
