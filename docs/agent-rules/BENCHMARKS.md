# Agent Rule – Benchmarks

## Scope

- Fixtures live under `testdata/tool-fixtures/<tool>/` with scenario definitions in `scenarios.json`.
- Scenarios MUST be command-driven against the declared project.
- Assertions MUST be deterministic (`must_contain`, `must_not_contain`, `required`, etc.).

## Expectations

- Primary KPI: token-oriented metrics.
- MUST track proxy overhead in milliseconds.
- MUST perform one best-effort native warmup before measured runs.
- MUST compare using cached `report.json`.
- Compaction-drop warnings only when scenario name and raw input hash match.
- Runtime artifacts MUST NOT be committed.
- Use `ccp gain`, not just the harness summary, to judge compression results for a tool.
- Highlight runs with no `gain.db` files or no meaningful compression result for the changed tool.

## Local Execution

- Use `./scripts/benchmark-local.sh -t <tool>` for tool-specific changes.
- SHOULD run the benchmark before changing an existing tool filter when a local baseline is useful.
- MUST run the benchmark after changing a tool filter or benchmarked execution behavior.
- Treat any non-zero benchmark exit as a failure.
- Compare before/after `ccp gain` output when a baseline exists.
