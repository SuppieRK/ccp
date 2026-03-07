package runner

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

const (
	errRegisterKubectlFmt   = "register kubectl: %v"
	errUnexpectedKubectlFmt = "unexpected error: %v"
)

func mustKubectlRegistry(t *testing.T) *engine.ToolFilterRegistry {
	t.Helper()
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewKubectlToolFilter()); err != nil {
		t.Fatalf(errRegisterKubectlFmt, err)
	}
	return reg
}

func TestBuildExecPlanKubectlAllowlistedGetPodsDispatches(t *testing.T) {
	reg := mustKubectlRegistry(t)
	plan, err := BuildExecPlan([]string{"kubectl", "get", "pods", "-o", "wide"}, reg)
	if err != nil {
		t.Fatalf(errUnexpectedKubectlFmt, err)
	}
	if plan.Tool != "kubectl" {
		t.Fatalf("expected kubectl tool, got %q", plan.Tool)
	}
	if plan.DispatchKey != "kubectl get pods" {
		t.Fatalf("expected kubectl get pods dispatch, got %q", plan.DispatchKey)
	}
}

func TestBuildExecPlanKubectlStructuredOutputPassthrough(t *testing.T) {
	reg := mustKubectlRegistry(t)
	plan, err := BuildExecPlan([]string{"kubectl", "get", "pods", "-o", "json"}, reg)
	if err != nil {
		t.Fatalf(errUnexpectedKubectlFmt, err)
	}
	if plan.Tool != "" {
		t.Fatalf("expected neutral passthrough tool binding, got %q", plan.Tool)
	}
	if plan.DispatchKey != "" {
		t.Fatalf("expected empty dispatch for passthrough, got %q", plan.DispatchKey)
	}
}

func TestBuildExecPlanKubectlPassthroughCases(t *testing.T) {
	reg := mustKubectlRegistry(t)
	cases := []struct {
		name string
		args []string
	}{
		{name: "logs-follow", args: []string{"kubectl", "logs", "pod-1", "-f"}},
		{name: "unsupported-subcommand", args: []string{"kubectl", "apply", "-f", "x.yaml"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildExecPlan(tc.args, reg)
			if err != nil {
				t.Fatalf(errUnexpectedKubectlFmt, err)
			}
			if plan.Tool != "" {
				t.Fatalf("expected passthrough tool binding, got %q", plan.Tool)
			}
		})
	}
}
