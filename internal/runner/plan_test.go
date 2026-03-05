package runner

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
)

const (
	errUnexpectedPlanTestFmt     = "unexpected error: %v"
	errRegisterPlanTestFmt       = "register: %v"
	errUnexpectedArgsPlanTestFmt = "unexpected args: want %#v, got %#v"
	errRegisterGitFilterPlanFmt  = "register git filter: %v"
)

var operatorChainArgs = []string{"echo", "a", "&&", "echo", "b"}

func TestBuildExecPlanDirect(t *testing.T) {
	plan, err := BuildExecPlan([]string{"echo", "hello"}, nil, false)
	if err != nil {
		t.Fatalf(errUnexpectedPlanTestFmt, err)
	}
	if plan.Name != "echo" {
		t.Fatalf("expected echo, got %q", plan.Name)
	}
	if len(plan.Args) != 1 || plan.Args[0] != "hello" {
		t.Fatalf("unexpected args: %#v", plan.Args)
	}
}

func TestBuildExecPlanShellForOperators(t *testing.T) {
	plan, err := BuildExecPlan(operatorChainArgs, nil, false)
	if err != nil {
		t.Fatalf(errUnexpectedPlanTestFmt, err)
	}
	if !plan.IsAmbiguous {
		t.Fatal("expected ambiguous plan for operators")
	}
	if plan.Tool != "" {
		t.Fatalf("expected neutral tool binding for ambiguous plan, got %q", plan.Tool)
	}
}

func TestBuildExecPlanLSPassthroughByDefault(t *testing.T) {
	registry := mustLSRegistry(t)
	plan, err := BuildExecPlan([]string{"ls"}, registry, false)
	if err != nil {
		t.Fatalf(errUnexpectedPlanTestFmt, err)
	}
	if plan.Name != "ls" {
		t.Fatalf("expected ls, got %q", plan.Name)
	}
	if len(plan.Args) != 0 {
		t.Fatalf("unexpected args: %#v", plan.Args)
	}
	if plan.Tool != "" {
		t.Fatalf("expected passthrough tool binding for default ls, got %q", plan.Tool)
	}
}

func TestBuildExecPlanLSNormalizesFlagsAndPaths(t *testing.T) {
	registry := mustLSRegistry(t)
	plan, err := BuildExecPlan([]string{"ls", "-lahR", "--all", "--color=always", "src"}, registry, false)
	if err != nil {
		t.Fatalf(errUnexpectedPlanTestFmt, err)
	}
	want := []string{"-la", "-R", "--color=always", "src"}
	if !slices.Equal(plan.Args, want) {
		t.Fatalf(errUnexpectedArgsPlanTestFmt, want, plan.Args)
	}
}

func TestBuildExecPlanUnknownToolFallback(t *testing.T) {
	plan, err := BuildExecPlan([]string{"unknown-tool", "--x", "y"}, engine.NewToolFilterRegistry(), false)
	if err != nil {
		t.Fatalf(errUnexpectedPlanTestFmt, err)
	}
	if plan.Tool != "unknown-tool" {
		t.Fatalf("expected fallback tool unknown-tool, got %q", plan.Tool)
	}
	want := []string{"--x", "y"}
	if !slices.Equal(plan.Args, want) {
		t.Fatalf(errUnexpectedArgsPlanTestFmt, want, plan.Args)
	}
}

func TestBuildExecPlanPropagatesPrepareAmbiguity(t *testing.T) {
	registry := mustAmbiguousPrepareRegistry(t)
	plan, err := BuildExecPlan([]string{"ambig", "a", "b"}, registry, false)
	if err != nil {
		t.Fatalf(errUnexpectedPlanTestFmt, err)
	}
	if !plan.IsAmbiguous {
		t.Fatal("expected ambiguous plan from Prepare")
	}
	if plan.AmbiguityReason != "prepare ambiguous" {
		t.Fatalf("unexpected ambiguity reason: %q", plan.AmbiguityReason)
	}
}

func TestBuildExecPlanStrictRejectsPrepareAmbiguity(t *testing.T) {
	registry := mustAmbiguousPrepareRegistry(t)
	_, err := BuildExecPlan([]string{"ambig", "a", "b"}, registry, true)
	if err == nil {
		t.Fatal("expected strict-mode error for prepare ambiguity")
	}
	if got := err.Error(); !containsAll(got, []string{"strict mode", "prepare ambiguous"}) {
		t.Fatalf("unexpected strict ambiguity diagnostics: %q", got)
	}
}

func TestBuildExecPlanGitUsesParentFilterAndSubcommandPrep(t *testing.T) {
	registry := mustGitRegistry(t)

	plan, err := BuildExecPlan([]string{"git", "status"}, registry, false)
	if err != nil {
		t.Fatalf(errUnexpectedPlanTestFmt, err)
	}
	if plan.Tool != "git" {
		t.Fatalf("expected git parent tool, got %q", plan.Tool)
	}
	if plan.DispatchKey != "git status" {
		t.Fatalf("expected dispatch key git status, got %q", plan.DispatchKey)
	}
	want := []string{"status", "--porcelain"}
	if !slices.Equal(plan.Args, want) {
		t.Fatalf(errUnexpectedArgsPlanTestFmt, want, plan.Args)
	}
}

func TestBuildExecPlanGitUnknownSubcommandFallsBackToPassthrough(t *testing.T) {
	registry := mustGitRegistry(t)

	plan, err := BuildExecPlan([]string{"git", "unknown", "--x"}, registry, false)
	if err != nil {
		t.Fatalf(errUnexpectedPlanTestFmt, err)
	}
	want := []string{"unknown", "--x"}
	if !slices.Equal(plan.Args, want) {
		t.Fatalf(errUnexpectedArgsPlanTestFmt, want, plan.Args)
	}
	if plan.Tool != "" {
		t.Fatalf("expected passthrough tool binding for unknown subcommand, got %q", plan.Tool)
	}
	if plan.DispatchKey != "" {
		t.Fatalf("expected empty dispatch key for unknown subcommand, got %q", plan.DispatchKey)
	}
}

func TestBuildExecPlanUnsupportedShapeFallsBackToPassthrough(t *testing.T) {
	registry := mustGitRegistry(t)

	plan, err := BuildExecPlan([]string{"git", "status", "&&", "git", "diff"}, registry, false)
	if err != nil {
		t.Fatalf(errUnexpectedPlanTestFmt, err)
	}
	if !plan.IsAmbiguous {
		t.Fatal("expected ambiguous plan for unsupported chain shape")
	}
	if plan.Tool != "" {
		t.Fatalf("expected neutral tool binding for ambiguous plan, got %q", plan.Tool)
	}
}

func mustLSRegistry(t *testing.T) *engine.ToolFilterRegistry {
	t.Helper()
	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewLSCompactor()); err != nil {
		t.Fatalf(errRegisterPlanTestFmt, err)
	}
	return registry
}

func mustGitRegistry(t *testing.T) *engine.ToolFilterRegistry {
	t.Helper()
	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(filters.NewGitToolFilter()); err != nil {
		t.Fatalf(errRegisterGitFilterPlanFmt, err)
	}
	return registry
}

func mustAmbiguousPrepareRegistry(t *testing.T) *engine.ToolFilterRegistry {
	t.Helper()
	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(ambiguousPrepareFilter{}); err != nil {
		t.Fatalf(errRegisterPlanTestFmt, err)
	}
	return registry
}

func TestBuildExecPlanForcePassthroughFromPrepare(t *testing.T) {
	registry := engine.NewToolFilterRegistry()
	if err := registry.Register(forcePassthroughFilter{runnerTestFilterBase: runnerTestFilterBase{tool: "git"}}); err != nil {
		t.Fatalf(errRegisterPlanTestFmt, err)
	}
	plan, err := BuildExecPlan([]string{"git", "diff", "--stat"}, registry, false)
	if err != nil {
		t.Fatalf(errUnexpectedPlanTestFmt, err)
	}
	if plan.Tool != "" {
		t.Fatalf("expected neutral passthrough tool, got %q", plan.Tool)
	}
	if plan.Name != "git" {
		t.Fatalf("expected direct binary, got %q", plan.Name)
	}
}

func TestBuildExecPlanStrictIncludesOperatorDiagnostics(t *testing.T) {
	_, err := BuildExecPlan(operatorChainArgs, nil, true)
	if err == nil {
		t.Fatal("expected strict-mode error")
	}
	if got := err.Error(); got == "" || !containsAll(got, []string{"strict mode", "&&"}) {
		t.Fatalf("expected diagnostics with operator, got %q", got)
	}
}

func TestBuildExecPlanDoesNotTreatInfixPipeArgumentAsAmbiguous(t *testing.T) {
	plan, err := BuildExecPlan([]string{"grep", "a|b", "file.txt"}, nil, false)
	if err != nil {
		t.Fatalf(errUnexpectedPlanTestFmt, err)
	}
	if plan.IsAmbiguous {
		t.Fatalf("did not expect ambiguous plan for infix pipe argument: %#v", plan)
	}
	if plan.Name != "grep" {
		t.Fatalf("expected direct execution, got %q", plan.Name)
	}
}

func containsAll(s string, parts []string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

type ambiguousPrepareFilter struct{}

func (ambiguousPrepareFilter) Tool() string { return "ambig" }
func (ambiguousPrepareFilter) Aliases() []string {
	return nil
}

func (ambiguousPrepareFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args, Ambiguous: true, Reason: "prepare ambiguous"}
}

func (ambiguousPrepareFilter) ContextKey(ev engine.Event) string {
	return runnerTestFilterBase{}.ContextKey(ev)
}

func (ambiguousPrepareFilter) Process(engine.Event, *engine.OrderedSetBuffer) engine.Decision {
	return engine.Decision{Action: engine.ActionCollect}
}
func (ambiguousPrepareFilter) MaskingHorizon() int { return 0 }

type forcePassthroughFilter struct {
	runnerTestFilterBase
}

func (f forcePassthroughFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
}
func (f forcePassthroughFilter) Process(engine.Event, *engine.OrderedSetBuffer) engine.Decision {
	return engine.Decision{Action: engine.ActionCollect}
}
