# Agent Rule – Benchmarks

## Scope

- Fixtures live under `testdata/benchmarks/<tool>/<case>-<invariant>/`.
- Replay fixtures are directory-driven and use `command.yaml` plus optional `stdout.txt`, `stderr.txt`, `output.txt`, and `decisions.txt`.
- `command.yaml` stores structured argv, and `stdout.txt` / `stderr.txt` keep sequenced `00000|` prefixes validated by `internal/replay`.
- Assertions default to exact `output.txt` equality when `output.txt` exists and exact `decisions.txt` equality when `decisions.txt` exists.

## Expectations

- Primary KPI: token-oriented metrics.
- Replay benchmarks compute token counts from concatenated sequenced `stdout.txt` + `stderr.txt`.
- Runtime artifacts MUST NOT be committed.
- Use `ccp gain`, not just the harness summary, to judge compression results for a tool.
- Highlight runs with no `gain.db` files or no meaningful compression result for the changed tool.

## Local Execution

- Use `go build -o .bin/ccp ./cmd/ccp && PATH="$PWD/.bin:$PATH" go run ./cmd/ccp-ci -tool <tool> -artifacts-dir .artifacts/benchmark-<tool>` for tool-specific changes.
- SHOULD run the benchmark before changing an existing tool filter when a local baseline is useful.
- MUST run the benchmark after changing a tool filter or benchmarked execution behavior.
- Treat any non-zero benchmark exit as a failure.
- Compare before/after `ccp gain` output when a baseline exists.
