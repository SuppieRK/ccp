## Purpose
Define safe planning and runtime compaction behavior for non-empty successful `git fetch` output.

## Requirements

### Requirement: Git Fetch Planning
The `git fetch` phase SHALL route standard fetch invocations through a dedicated compactor.

#### Scenario: fetch dispatch
- **WHEN** `git fetch` is invoked
- **THEN** the `git` parent dispatches to the `git fetch` subfilter.

#### Scenario: explicit detail passthrough
- **WHEN** `git fetch` args request explicit detail or machine-readable output such as `--verbose`, `--dry-run`, or `--porcelain`
- **THEN** the `git fetch` phase returns passthrough behavior.

### Requirement: Git Fetch Runtime Handling
The `git fetch` phase SHALL preserve actionable update and error signal while reducing repetitive transfer noise.

#### Scenario: successful fetch summary
- **WHEN** `git fetch` completes successfully with non-empty update output
- **THEN** compacted output includes a stable summary of fetched updates or completion state.

#### Scenario: empty successful fetch remains empty
- **WHEN** `git fetch` completes successfully and produces no native output
- **THEN** the phase emits no synthetic output.

#### Scenario: diagnostics and exit parity
- **WHEN** `git fetch` emits diagnostics or exits non-zero
- **THEN** diagnostics remain visible and native exit-code parity is preserved.

#### Scenario: low-confidence fallback
- **WHEN** `git fetch` output cannot be compacted with sufficient confidence
- **THEN** the phase falls back to passthrough output.
