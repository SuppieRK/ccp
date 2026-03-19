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
- Corpus honesty matters as much as savings. Prefer widening fixtures with real output before making strong claims from tiny or toy scenarios.
- `stdout.txt` and `stderr.txt` must start at `00000|` and remain contiguous; replay treats broken or one-based numbering as an error.
- When a fixture drifts, prefer promoting the verified runtime output (`verify-output.txt`) or regenerating it with `ccp capture` instead of rewriting expected output from memory.

## Local Execution

- Use `go build -o .bin/ccp ./cmd/ccp && PATH="$PWD/.bin:$PATH" go run ./cmd/ccp-ci -tool <tool> -artifacts-dir .artifacts/benchmark-<tool>` for tool-specific changes.
- SHOULD run the benchmark before changing an existing tool filter when a local baseline is useful.
- MUST run the benchmark after changing a tool filter or benchmarked execution behavior.
- Treat any non-zero benchmark exit as a failure.
- Compare before/after `ccp gain` output when a baseline exists.

## Fixture Strategy

- Add boundary fixtures, not just winning fixtures. Passthrough cases are valuable when they prove a filter is intentionally conservative.
- Prefer real warning-bearing success, rich failure, and machine-mode examples over duplicated clean-success variants.
- If a command family has user-defined content such as logs, benchmark it only when CCP is truly expected to leave it native or when the tool itself provides a safe structured mode.
