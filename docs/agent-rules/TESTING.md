# Agent Rule – Testing

## Skills

- Prefer a Test-Driven Development workflow: write or update the failing test first, then implement, then bring the suite back to green.
- Use the `bdd` skill when adding or changing tests that use Ginkgo or Gomega.
- Prefer Ginkgo/Gomega BDD-style tests because they are easier for humans to read and faster to understand during development and review.
- When an agent encounters tests during implementation, it must refactor the touched tests to Ginkgo/Gomega BDD style as part of the same change.
- If a touched test cannot be converted safely in the same change, the agent must call out the blocker explicitly and treat the work as incomplete rather than leaving mixed styles without explanation.
- New test coverage should default to Ginkgo/Gomega BDD style unless a specific package or constraint requires a different test shape.

## Test Selection

- Planning changes affect outputs such as tool/name resolution, args, dispatch keys, ambiguity handling, and passthrough decisions.
- Runtime changes affect `Runner.Run` behavior such as stdout/stderr handling, shared-context behavior, compaction vs passthrough, raw-mode bypass, and exit-code propagation.
- Filter-specific runner, benchmark, and fixture expectations belong in `docs/agent-rules/FILTERS.md`.

## Test Placement

- Command-specific runtime: `internal/*_<command>_test.go` or the narrowest owning package
- Generic runtime: `internal/runner_test.go`, `internal/engine/*_test.go`, `internal/filters/*_test.go`, or the matching owning package
- Fixture replay integration: benchmark/replay coverage under `internal/benchmark` plus `testdata/benchmarks/`
- Shared planner/runtime scaffolding: helper files near the owning package rather than a parallel compatibility tree
- Recipe-backed lifecycle adapters: family conformance tests in `internal/lifecycle/agents/*conformance*_test.go`, with bespoke adapter tests only where custom behavior remains

Command-specific behavior MUST NOT be placed in generic test files.

## Expectations

- Add or update tests in the narrowest layer that exercises the changed behavior.
- Prefer command-scoped tests over expanding generic suites when behavior is tool-specific.
- Prefer source-oriented suites with shared table/helper coverage over repetitive wrapper-only tests.
- `./scripts/validate.sh` is the canonical local validation command and must pass before changes are complete.
