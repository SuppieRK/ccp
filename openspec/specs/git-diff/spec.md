## Purpose
Define `git diff` filter behavior.

## Requirements

### Requirement: Diff Prepare Escape Hatches
`git diff` SHALL expose passthrough escape hatches in prepare phase.

#### Scenario: No-compact flag
- **WHEN** args contain `--no-compact`
- **THEN** passthrough is forced
- **AND** `--no-compact` is removed from underlying git args.

#### Scenario: Stat-only flags
- **WHEN** args contain `--stat`, `--numstat`, or `--shortstat`
- **THEN** passthrough is forced.

### Requirement: Diff Runtime Compaction
`git diff` SHALL compact stdout diff bodies and passthrough stderr lines immediately.

#### Scenario: Stderr line
- **WHEN** stderr line is received
- **THEN** it is emitted immediately.

#### Scenario: EOF with diff body
- **WHEN** stdout reaches EOF with diff content
- **THEN** output includes per-file `+/-` totals, bounded snippets, and `summary: <n> files changed, +<a> -<d>`.
- **AND** snippets include only hunk change lines (`+...` / `-...`, excluding file header markers) with at most 6 snippet lines per file hunk segment.
