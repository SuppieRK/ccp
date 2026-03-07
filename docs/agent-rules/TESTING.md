# Agent Rule – Testing

## Test Selection

- Planning changes affect `BuildExecPlan` outputs such as tool/name resolution, args, dispatch keys, ambiguity handling, and passthrough decisions.
- Runtime changes affect `Runner.Run` behavior such as stdout/stderr handling, shared-context behavior, compaction vs passthrough, raw-mode bypass, and exit-code propagation.
- Filter-specific runner, benchmark, and fixture expectations belong in `docs/agent-rules/FILTERS.md`.

## Test Placement

- Command-specific planning: `internal/runner/plan_<command>_test.go`
- Generic planning: `internal/runner/plan_test.go`
- Command-specific runtime: `internal/runner/runner_<command>_integration_test.go`
- Generic runtime: `internal/runner/runner_test.go`
- Fixture replay integration: `internal/engine/filters/tool_fixtures_integration_test.go`

Command-specific behavior MUST NOT be placed in generic test files.

## Expectations

- Add or update tests in the narrowest layer that exercises the changed behavior.
- Prefer command-scoped tests over expanding generic suites when behavior is tool-specific.
- `./scripts/validate.sh` is the canonical local validation command and must pass before changes are complete.
