package runner

import "testing"

func TestBuildExecPlanGitShowDispatchesDefaultShape(t *testing.T) {
	reg := mustGitRegistry(t)

	plan, err := BuildExecPlan([]string{"git", "show", "HEAD"}, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Tool != "git" {
		t.Fatalf("expected git parent tool, got %q", plan.Tool)
	}
	if plan.DispatchKey != "git show" {
		t.Fatalf("expected git show dispatch, got %q", plan.DispatchKey)
	}
	if len(plan.Args) < 2 || plan.Args[0] != "show" || plan.Args[1] != "HEAD" {
		t.Fatalf("unexpected normalized args: %#v", plan.Args)
	}
}

func TestBuildExecPlanGitShowPrecisionSensitiveShapesPassthrough(t *testing.T) {
	reg := mustGitRegistry(t)

	tests := []struct {
		name string
		args []string
	}{
		{name: "format", args: []string{"git", "show", "--format=%H"}},
		{name: "stat", args: []string{"git", "show", "--stat"}},
		{name: "blob", args: []string{"git", "show", "HEAD:README.md"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildExecPlan(tc.args, reg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if plan.Tool != "" {
				t.Fatalf("expected passthrough tool binding, got %#v", plan)
			}
			if plan.DispatchKey != "git show" {
				t.Fatalf("expected delegated git show dispatch, got %q", plan.DispatchKey)
			}
		})
	}
}
