package runner

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

const errUnexpectedPlanFmt = "unexpected error: %v"

func mustDockerRegistry(t *testing.T) *engine.ToolFilterRegistry {
	t.Helper()
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewDockerToolFilter()); err != nil {
		t.Fatalf("register docker: %v", err)
	}
	return reg
}

func TestBuildExecPlanDockerDispatchesPSImagesLogs(t *testing.T) {
	reg := mustDockerRegistry(t)
	cases := []struct {
		name     string
		args     []string
		dispatch string
	}{
		{name: "docker ps", args: []string{"docker", "ps"}, dispatch: "docker ps"},
		{name: "docker images", args: []string{"docker", "images"}, dispatch: "docker images"},
		{name: "docker compose logs", args: []string{"docker", "compose", "logs", "--tail", "20", "api"}, dispatch: "docker compose logs|scope=api"},
		{name: "docker compose ps", args: []string{"docker", "compose", "ps"}, dispatch: "docker compose ps"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildExecPlan(tc.args, reg)
			if err != nil {
				t.Fatalf(errUnexpectedPlanFmt, err)
			}
			if plan.Tool != "docker" || plan.DispatchKey != tc.dispatch {
				t.Fatalf("unexpected plan: %#v", plan)
			}
		})
	}
}

func TestBuildExecPlanDockerLogsDispatchesContainerScope(t *testing.T) {
	reg := mustDockerRegistry(t)

	logPlan, err := BuildExecPlan([]string{"docker", "logs", "--tail", "20", "api"}, reg)
	if err != nil {
		t.Fatalf(errUnexpectedPlanFmt, err)
	}
	if logPlan.Tool != "docker" || !strings.HasPrefix(logPlan.DispatchKey, "docker logs|container=api") {
		t.Fatalf("unexpected docker logs plan: %#v", logPlan)
	}
}

func TestBuildExecPlanDockerPassthroughBoundaries(t *testing.T) {
	reg := mustDockerRegistry(t)

	tests := [][]string{
		{"docker", "compose", "ps", "--format", "json"},
		{"docker", "exec", "-it", "c1", "sh"},
		{"docker", "pull", "nginx"},
		{"docker", "build", "."},
		{"docker", "run", "--rm", "alpine", "echo", "ok"},
		{"docker", "logs", "api", "-f"},
		{"docker", "compose", "logs", "api", "--follow"},
	}
	for _, tc := range tests {
		plan, err := BuildExecPlan(tc, reg)
		if err != nil {
			t.Fatalf("unexpected error for %#v: %v", tc, err)
		}
		if plan.Tool != "" {
			t.Fatalf("expected neutral passthrough for %#v, got %#v", tc, plan)
		}
	}
}
