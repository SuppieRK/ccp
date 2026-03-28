# Agent Rule – Testing

## Default Test Style

- Use the `bdd` skill when adding or changing Ginkgo/Gomega coverage.
- New test coverage should default to Ginkgo/Gomega BDD style unless a package or constraint clearly requires otherwise.
- Prefer readable `Describe` / `Context` / `It` structure and `DescribeTable` for repeated cases.
- When touching older non-BDD tests, prefer moving them toward the surrounding package style if that can be done safely
  in the same change.

## Test Selection

- Add or update tests in the narrowest layer that exercises the changed behavior.
- Prefer command-scoped or package-scoped tests over expanding generic suites when behavior is tool-specific.
- Planner or dispatch changes belong near the owning planner/runtime code.
- Runtime behavior changes belong near the owning runner, replay, filter-runtime, or lifecycle package.
- Filter-specific fixture and benchmark expectations belong in `docs/agent-rules/FILTERS.md` and
  `docs/agent-rules/BENCHMARKS.md`.

## Test Placement

- Keep tests near the owning package.
- Shared helpers should live near the package that owns the behavior, not in a parallel compatibility tree.
- Benchmark and replay coverage lives in `internal/benchmark`, `internal/replay`, `internal/lifecycle`, and
  `testdata/benchmarks/` depending on what changed.
- Lifecycle adapter tests should prefer shared family/conformance coverage, with bespoke adapter tests only where
  bespoke behavior remains.
- Command-specific behavior should not be hidden inside generic runtime test files.

## Expectations

- Behavioral changes need matching tests or fixture updates.
- Prefer source-oriented suites with shared helpers over repetitive wrapper-only tests.
- `./scripts/validate.sh` is the canonical local validation command and must pass before the work is complete.
- Treat the internal coverage gate enforced by `./scripts/validate.sh` as part of the testing contract, not an optional
  extra.
