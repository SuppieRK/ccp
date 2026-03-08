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

	psPlan, err := BuildExecPlan([]string{"docker", "ps"}, reg)
	if err != nil {
		t.Fatalf(errUnexpectedPlanFmt, err)
	}
	if psPlan.Tool != "docker" || psPlan.DispatchKey != "docker ps" {
		t.Fatalf("unexpected docker ps plan: %#v", psPlan)
	}

	imgPlan, err := BuildExecPlan([]string{"docker", "images"}, reg)
	if err != nil {
		t.Fatalf(errUnexpectedPlanFmt, err)
	}
	if imgPlan.Tool != "docker" || imgPlan.DispatchKey != "docker images" {
		t.Fatalf("unexpected docker images plan: %#v", imgPlan)
	}

	logPlan, err := BuildExecPlan([]string{"docker", "logs", "--tail", "20", "api"}, reg)
	if err != nil {
		t.Fatalf(errUnexpectedPlanFmt, err)
	}
	if logPlan.Tool != "docker" || !strings.HasPrefix(logPlan.DispatchKey, "docker logs|container=api") {
		t.Fatalf("unexpected docker logs plan: %#v", logPlan)
	}

	composeLogPlan, err := BuildExecPlan([]string{"docker", "compose", "logs", "--tail", "20", "api"}, reg)
	if err != nil {
		t.Fatalf(errUnexpectedPlanFmt, err)
	}
	if composeLogPlan.Tool != "docker" || composeLogPlan.DispatchKey != "docker compose logs|scope=api" {
		t.Fatalf("unexpected docker compose logs plan: %#v", composeLogPlan)
	}
}

func TestBuildExecPlanDockerPassthroughBoundaries(t *testing.T) {
	reg := mustDockerRegistry(t)

	tests := [][]string{
		{"docker", "compose", "ps"},
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
