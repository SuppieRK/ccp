package runner

import "testing"

func TestBuildExecPlanGitFetchDispatchesStandardShapes(t *testing.T) {
	reg := mustGitRegistry(t)

	tests := []struct {
		name string
		args []string
	}{
		{name: "default", args: []string{"git", "fetch"}},
		{name: "remote", args: []string{"git", "fetch", "origin"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildExecPlan(tc.args, reg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if plan.Tool != "git" {
				t.Fatalf("expected git parent tool, got %#v", plan)
			}
			if plan.DispatchKey != "git fetch" {
				t.Fatalf("expected git fetch dispatch, got %q", plan.DispatchKey)
			}
		})
	}
}

func TestBuildExecPlanGitFetchDetailShapesPassthrough(t *testing.T) {
	reg := mustGitRegistry(t)

	tests := []struct {
		name string
		args []string
	}{
		{name: "verbose", args: []string{"git", "fetch", "--verbose"}},
		{name: "dry-run", args: []string{"git", "fetch", "--dry-run", "origin"}},
		{name: "porcelain", args: []string{"git", "fetch", "--porcelain"}},
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
			if plan.DispatchKey != "git fetch" {
				t.Fatalf("expected delegated git fetch dispatch, got %q", plan.DispatchKey)
			}
		})
	}
}
