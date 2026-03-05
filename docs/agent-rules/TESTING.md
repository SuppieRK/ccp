# Agent Rule – Testing

This file defines mandatory testing structure rules.

## Planning vs Runtime Changes

Planning change:
- Affects `BuildExecPlan` outputs:
  Tool, Name, Args, DispatchKey,
  ambiguity handling,
  strict behavior,
  passthrough decisions.

Runtime change:
- Affects `Runner.Run` behavior:
  stdout/stderr handling,
  shared-context behavior,
  compaction vs passthrough,
  raw-mode bypass,
  exit-code propagation.

## Test Placement

- Command-specific planning: `internal/runner/plan_<command>_test.go`
- Generic planning: `internal/runner/plan_test.go`
- Command-specific runtime: `internal/runner/runner_<command>_integration_test.go`
- Generic runtime: `internal/runner/runner_test.go`

Command-specific behavior MUST NOT be placed in generic test files.

## Coverage Gate

`cmd/coverage-gate` MUST pass in CI for all pull requests.