# Agent Rule – Benchmarks

## Fixture Model

- Replay benchmark fixtures live under `testdata/benchmarks/<tool>/<case>/`.
- A fixture is runnable when it contains `command.yaml`.
- `command.yaml` stores argv and optional `exit_code`.
- Fixture files are text-driven:
  - optional `stdout.txt`
  - optional `stderr.txt`
  - optional `output.txt`
  - optional `decisions.txt`
- At least one of `stdout.txt`, `stderr.txt`, or `output.txt` must exist.
- `stdout.txt` and `stderr.txt` use contiguous zero-based `00000|` prefixes enforced by `internal/replay`.
- When present, `output.txt` and `decisions.txt` are exact expectations checked against `verify-output.txt` and `verify-decisions.txt`.

## Runner Model

- `cmd/ccp-ci` is a thin wrapper over `internal/benchmark.Run`.
- The runner discovers fixture directories, copies each case into an artifact directory, and runs `ccp verify --dir <artifact-dir>`.
- Native token count comes from merged sequenced `stdout.txt` + `stderr.txt`.
- Proxy token count comes from `verify-output.txt`.
- Per-case metrics are written to `<artifact-dir>/.ccp/gain.db`.
- Read `ccp gain` from the case artifact directory, not from the parent benchmark directory.

## Working Rules

- Primary KPI: token-oriented compaction from real replay fixtures.
- Corpus honesty matters as much as savings. Add boundary and passthrough cases, not just wins.
- Prefer widening real corpus before redesigning a filter around tiny or toy fixtures.
- When a fixture drifts, promote `verify-output.txt` / `verify-decisions.txt` or regenerate with `ccp capture` instead of rewriting expectations from memory.
- Treat missing `gain.db`, missing `verify-output.txt`, output mismatches, decision mismatches, or any non-zero benchmark exit as failures to investigate.
- Use `ccp gain`, not just the harness summary, to judge compression for the changed tool.
- Benchmark logs or other user-defined output only when the intended result is native passthrough or the tool provides a safe structured mode.

## Local Verification

Run the benchmark-related Ginkgo/Gomega suites when changing replay fixtures, benchmark harness behavior, `ccp verify`, or tool filters:

```bash
go test -count=1 ./internal/benchmark ./cmd/ccp-ci ./internal/lifecycle
```

Full contributor validation remains:

```bash
./scripts/validate.sh
```
