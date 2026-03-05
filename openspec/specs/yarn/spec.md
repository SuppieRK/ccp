## Purpose
Define `yarn` filter identity, prepare routing, shared-context runtime handling, and npm-compatible compaction.

## Requirements

### Requirement: Yarn Tool Identity And Aliases
The `yarn` filter SHALL identify as `yarn` and support `yarnpkg` alias executables.

#### Scenario: alias executables
- **WHEN** executable is `yarnpkg`, `yarn.cmd`, `./yarn.cmd`, `yarn.exe`, `./yarn.exe`, `yarnpkg.cmd`, `./yarnpkg.cmd`, `yarnpkg.exe`, or `./yarnpkg.exe`
- **THEN** the `yarn` filter contract is used.

### Requirement: Prepare Dispatch Behavior
The filter SHALL preserve args and assign run/passthrough dispatch mode.

#### Scenario: no-args passthrough
- **WHEN** yarn is invoked without args
- **THEN** prepare returns passthrough with preserved args.

#### Scenario: run-mode dispatch
- **WHEN** first arg is `run` (case-insensitive)
- **THEN** dispatch key is `yarn|mode=run`.

#### Scenario: non-run dispatch
- **WHEN** first arg is not `run`
- **THEN** dispatch key is `yarn|mode=passthrough`.

### Requirement: Shared Runtime Context
Yarn runtime SHALL use a shared context across stdout/stderr streams.

#### Scenario: shared stream context
- **WHEN** stdout and stderr events belong to the same command
- **THEN** both streams resolve to one shared context key.

### Requirement: Runtime Compaction Behavior
Yarn runtime SHALL collect pre-exit output and decide flush behavior on exit.

#### Scenario: collect-then-exit handling
- **WHEN** line, tick, or EOF events arrive
- **THEN** output is collected and final decision is made on exit.

#### Scenario: empty successful output
- **WHEN** exit is zero and buffered output is empty
- **THEN** output is `ok`.

#### Scenario: empty failing output
- **WHEN** exit is non-zero and buffered output is empty
- **THEN** no output is emitted.

#### Scenario: compacted non-empty output
- **WHEN** buffered output is present and npm-compatible compaction yields non-empty output
- **THEN** compacted output is flushed.

#### Scenario: compaction fallback
- **WHEN** compaction is low-confidence
- **THEN** raw buffered output is flushed unchanged.

#### Scenario: empty compacted output fallback
- **WHEN** compaction returns empty output for non-empty raw buffer
- **THEN** `ok` is emitted when exit is zero.
- **AND** raw buffered output is flushed when exit is non-zero.
