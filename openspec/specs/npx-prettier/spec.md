## Purpose
Define `npx prettier` subfilter prepare and runtime summarization behavior.

## Requirements

### Requirement: prettier Prepare Passthrough
The `npx prettier` route SHALL preserve input args in prepare.

#### Scenario: prepare preserves args
- **WHEN** prepare runs for `npx prettier`
- **THEN** normalized args are identical to input args.

### Requirement: prettier Runtime Stream Handling
The `npx prettier` route SHALL collect stdout through exit and summarize or fallback at exit.

#### Scenario: stdout pre-exit collection
- **WHEN** stdout receives line/tick events
- **THEN** events are collected.

#### Scenario: stdout EOF behavior
- **WHEN** stdout EOF is received before exit
- **THEN** no output is emitted.

#### Scenario: exit with empty or wrapper-only output
- **WHEN** exit is received and buffered output is empty after wrapper-noise stripping
- **THEN** no output is emitted.

### Requirement: prettier Summary Modes
The `npx prettier` route SHALL summarize recognized check/write outputs and fallback safely.

#### Scenario: wrapper-noise suppression
- **WHEN** buffered stdout includes npx wrapper boilerplate
- **THEN** wrapper lines are removed before summarization.

#### Scenario: check success summary
- **WHEN** output matches `Checking formatting...` + `All matched files use Prettier code style!`
- **THEN** output is `prettier check: ok`.

#### Scenario: check failure summary
- **WHEN** output reports code-style issues with recognizable file paths
- **THEN** output is `prettier check: <N> files need formatting` plus file list.
- **AND** recognized warning-path lines (`[warn] <path>`) contribute to that file list.

#### Scenario: write summary
- **WHEN** multiple formatted-path timing lines are recognized
- **THEN** output is `prettier write: formatted <N> files` plus file list.

#### Scenario: unknown-output fallback
- **WHEN** structured summary confidence is low
- **THEN** stripped raw output is flushed unchanged.
