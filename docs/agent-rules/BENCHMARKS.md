# Agent Rule – Benchmarks

## Structure

- Assets under: `testdata/tool-fixtures/<tool>/`
- Scenario definitions in: `scenarios.json`
- Scenarios MUST be command-driven against declared project.
- MUST use deterministic assertions:
  `must_contain`
  `must_not_contain`
- MUST use `required: true` for fail-fast.

## Determinism

- Primary KPI: token-oriented metrics.
- MUST track proxy overhead in milliseconds.
- MUST perform one best-effort native warmup before measured runs.
- MUST compare using cached `report.json`.
- Compaction-drop warnings only when scenario name and raw input hash match.
- Runtime artifacts MUST NOT be committed.

## Local Execution

```shell
./scripts/benchmark-local.sh
```