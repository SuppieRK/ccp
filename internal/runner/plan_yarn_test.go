package runner

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

func TestBuildExecPlanYarnAliasResolvesToYarnFilter(t *testing.T) {
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewYarnFilter()); err != nil {
		t.Fatalf("register: %v", err)
	}

	for _, bin := range []string{"yarn", "yarnpkg"} {
		plan, err := BuildExecPlan([]string{bin, "install"}, reg)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", bin, err)
		}
		if plan.Tool != "yarn" {
			t.Fatalf("expected yarn tool binding for %s, got %q", bin, plan.Tool)
		}
		if plan.Name != bin {
			t.Fatalf("expected original binary preserved for %s, got %q", bin, plan.Name)
		}
		if plan.DispatchKey != "yarn|mode=passthrough" {
			t.Fatalf("expected passthrough dispatch mode for %s, got %q", bin, plan.DispatchKey)
		}
	}
}
