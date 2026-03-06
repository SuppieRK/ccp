# Agent Rule – Filters

## Architecture

- Filters implement `engine.ToolFilter` (`Tool`, `Aliases`, `Prepare`, `ContextKey`, `Process`, `MaskingHorizon`).
- Single-command tools typically live in a top-level file such as `internal/engine/filters/ls.go`.
- Parent-command tools typically use a top-level delegating filter plus `ToolFilterRegistry`-backed subcommands under `internal/engine/filters/<parent>/`.
- Shared parent-tool delegation helpers typically live in top-level files such as `parent_delegate.go`.

## Layout Guidance

- Keep files responsibility-scoped, but do not force a strict one-file-per-tool rule where the implementation is clearer split across multiple files.
- Keep matching tests close to the implementation: top-level filters in `internal/engine/filters/*_test.go`, subcommand filters in `internal/engine/filters/<parent>/*_test.go`.
- Prefer `common.go` or `helpers.go` for shared helper code.
- Reuse existing shared helper files where they already exist, but avoid introducing new helper-file names unless the scope is clearly distinct and justified.
- Parent-command routing SHOULD use `ToolFilterRegistry` via helpers like `newSubcommandRegistry`.

## Spec Alignment

- Parent-command tools require parent and subcommand specs.
- Specs under: `openspec/specs/`
- Spec-fixture directory names MUST match spec IDs.
- When specs change, update: `internal/engine/filters/tool_fixtures_integration_test.go` (`toolSpecNames`).

## Benchmark Coverage

- Follow `docs/agent-rules/BENCHMARKS.md` for benchmark workflow and expectations.
- New non-parent filters MUST add benchmark fixtures under `testdata/tool-fixtures/<tool>/`.
- Existing non-parent filters with behavior changes MUST update benchmark coverage for the changed behavior.
- Parent routing filters are exempt unless the parent filter itself owns benchmarked output behavior.

## Runner Test Coverage

- Follow `docs/agent-rules/TESTING.md` for test-layer selection and generic placement rules.
- New filters MUST add command-specific runner tests using `internal/runner/plan_<tool>_test.go` and `internal/runner/runner_<tool>_integration_test.go`.
- Existing filters with planning or runtime behavior changes MUST update those runner tests alongside filter tests.
- Parent routing filters are exempt unless they change planner or runner-visible behavior owned by the parent filter itself.
