package runner

import (
	"errors"
	"os/exec"
	"slices"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

const (
	errRegisterPIPFmt   = "register: %v"
	errUnexpectedPIPFmt = "unexpected error: %v"
)

func resetLookPathCacheForTest() {
	lookPathMu.Lock()
	lookPathOK = map[string]bool{}
	lookPathMu.Unlock()
}

func setLookPathFnForTest(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	old := lookPathFn
	lookPathFn = fn
	resetLookPathCacheForTest()
	t.Cleanup(func() {
		lookPathFn = old
	})
}

func mustPIPRegistry(t *testing.T) *engine.ToolFilterRegistry {
	t.Helper()
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(filters.NewPIPFilter()); err != nil {
		t.Fatalf(errRegisterPIPFmt, err)
	}
	return reg
}

func TestBuildExecPlanPIPAliasResolvesAndStructuredArgs(t *testing.T) {
	reg := mustPIPRegistry(t)

	setLookPathFnForTest(t, func(file string) (string, error) { return "", exec.ErrNotFound })

	plan, err := BuildExecPlan([]string{"pip3", "list"}, reg)
	if err != nil {
		t.Fatalf(errUnexpectedPIPFmt, err)
	}
	if plan.Tool != "pip" {
		t.Fatalf("expected pip tool binding, got %q", plan.Tool)
	}
	if plan.DispatchKey != "pip|mode=list" {
		t.Fatalf("unexpected dispatch key: %q", plan.DispatchKey)
	}
	if plan.Name != "pip3" {
		t.Fatalf("expected fallback binary name preserved, got %q", plan.Name)
	}
	if !slices.Equal(plan.Args, []string{"list", "--format=json"}) {
		t.Fatalf("unexpected args: %#v", plan.Args)
	}
}

func TestBuildExecPlanPIPUsesUVPreferredSubstitutionWhenAvailable(t *testing.T) {
	reg := mustPIPRegistry(t)

	calls := 0
	setLookPathFnForTest(t, func(file string) (string, error) {
		calls++
		if file == "uv" {
			return "/usr/bin/uv", nil
		}
		return "", errors.New("not found")
	})

	plan, err := BuildExecPlan([]string{"pip", "outdated"}, reg)
	if err != nil {
		t.Fatalf(errUnexpectedPIPFmt, err)
	}
	if plan.Name != "uv" {
		t.Fatalf("expected uv substitution, got %q", plan.Name)
	}
	if !slices.Equal(plan.Args, []string{"pip", "list", "--outdated", "--format=json"}) {
		t.Fatalf("unexpected substituted args: %#v", plan.Args)
	}
	if calls != 1 {
		t.Fatalf("expected one lookPath call, got %d", calls)
	}
}

func TestBuildExecPlanPIPUVLookupCachedAcrossPlans(t *testing.T) {
	reg := mustPIPRegistry(t)

	calls := 0
	setLookPathFnForTest(t, func(file string) (string, error) {
		calls++
		if file == "uv" {
			return "/usr/bin/uv", nil
		}
		return "", exec.ErrNotFound
	})

	if _, err := BuildExecPlan([]string{"pip", "list"}, reg); err != nil {
		t.Fatalf(errUnexpectedPIPFmt, err)
	}
	if _, err := BuildExecPlan([]string{"pip", "outdated"}, reg); err != nil {
		t.Fatalf(errUnexpectedPIPFmt, err)
	}
	if calls != 1 {
		t.Fatalf("expected cached uv lookup (1 call), got %d", calls)
	}
}

func TestBuildExecPlanPIPAmbiguousPassthroughCases(t *testing.T) {
	reg := mustPIPRegistry(t)
	cases := []struct {
		name     string
		args     []string
		wantName string
	}{
		{
			name: "format-conflict",
			args: []string{"pip", "list", "--format", "freeze"},
		},
		{
			name:     "compatibility-sensitive-flags",
			args:     []string{"pip", "install", "--editable", "."},
			wantName: "pip",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildExecPlan(tc.args, reg)
			if err != nil {
				t.Fatalf(errUnexpectedPIPFmt, err)
			}
			if !plan.IsAmbiguous || plan.Tool != "" {
				t.Fatalf("expected ambiguous neutral passthrough plan, got %#v", plan)
			}
			if tc.wantName != "" && plan.Name != tc.wantName {
				t.Fatalf("expected original command preserved, got %q", plan.Name)
			}
		})
	}
}

func TestBuildExecPlanDirectUVShapeRemainsPassthrough(t *testing.T) {
	reg := mustPIPRegistry(t)
	if resolved := reg.Resolve("uv"); resolved != nil {
		t.Fatalf("expected no dedicated uv tool filter, got %q", resolved.Tool())
	}
	plan, err := BuildExecPlan([]string{"uv", "run", "python", "-V"}, reg)
	if err != nil {
		t.Fatalf(errUnexpectedPIPFmt, err)
	}
	if plan.Tool != "uv" {
		t.Fatalf("expected noop passthrough tool binding for direct uv, got %q", plan.Tool)
	}
	if plan.Name != "uv" {
		t.Fatalf("expected direct uv executable preserved, got %q", plan.Name)
	}
	if !slices.Equal(plan.Args, []string{"run", "python", "-V"}) {
		t.Fatalf("unexpected direct uv args: %#v", plan.Args)
	}
}

func TestRunnerInitializationPrimesUVLookupCache(t *testing.T) {
	calls := 0
	setLookPathFnForTest(t, func(file string) (string, error) {
		calls++
		return "", exec.ErrNotFound
	})

	reg := engine.NewToolFilterRegistry()
	_ = New(Options{}, nil, reg)
	_ = New(Options{}, nil, reg)
	if calls != 1 {
		t.Fatalf("expected initialization-scoped cached lookup, got %d calls", calls)
	}
}
