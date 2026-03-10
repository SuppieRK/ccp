package runner

import "testing"

func TestBuildExecPlanGitWorktreeListDispatches(t *testing.T) {
	reg := mustGitRegistry(t)

	plan, err := BuildExecPlan([]string{"git", "worktree", "list"}, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Tool != "git" {
		t.Fatalf("expected git parent tool, got %q", plan.Tool)
	}
	if plan.DispatchKey != "git worktree" {
		t.Fatalf("expected git worktree dispatch, got %q", plan.DispatchKey)
	}
	if plan.Passthrough {
		t.Fatalf("did not expect passthrough: %#v", plan)
	}
}

func TestBuildExecPlanGitWorktreeManagementAndPrecisionShapesPassthrough(t *testing.T) {
	reg := mustGitRegistry(t)

	tests := []struct {
		name string
		args []string
	}{
		{name: "add", args: []string{"git", "worktree", "add", "../wt"}},
		{name: "remove", args: []string{"git", "worktree", "remove", "../wt"}},
		{name: "verbose list", args: []string{"git", "worktree", "list", "--verbose"}},
		{name: "porcelain list", args: []string{"git", "worktree", "list", "--porcelain"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildExecPlan(tc.args, reg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if plan.Tool != "" {
				t.Fatalf("expected neutral tool binding for passthrough shape, got %#v", plan)
			}
			if plan.MetricsTool != "git" || !plan.Passthrough {
				t.Fatalf("expected canonical git metrics identity for passthrough, got %#v", plan)
			}
			if plan.DispatchKey != "git worktree" {
				t.Fatalf("expected delegated git worktree dispatch, got %q", plan.DispatchKey)
			}
		})
	}
}
