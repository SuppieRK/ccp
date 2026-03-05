## Purpose
Define `npx tsc` subfilter prepare defaults and runtime diagnostic summarization behavior.

## Requirements

### Requirement: tsc Prepare Pretty Normalization
The `npx tsc` route SHALL disable pretty rendering by default.

#### Scenario: prepare pretty injection
- **WHEN** args do not include `--pretty`
- **THEN** `--pretty false` is appended.

#### Scenario: explicit pretty preserved
- **WHEN** args already include `--pretty`
- **THEN** args are preserved.

### Requirement: tsc Runtime Stream Handling
The `npx tsc` route SHALL preserve stderr immediacy and process stdout at exit.

#### Scenario: stderr immediate visibility
- **WHEN** stderr line events are received
- **THEN** lines are emitted immediately unchanged.

#### Scenario: stdout pre-exit collection
- **WHEN** stdout receives tick/line events
- **THEN** events are collected.

#### Scenario: stdout EOF behavior
- **WHEN** stdout EOF is received before exit
- **THEN** no output is emitted.

#### Scenario: empty or wrapper-only output
- **WHEN** exit is received and buffered output is empty after wrapper-noise stripping
- **THEN** no output is emitted.

### Requirement: tsc Diagnostic Summarization
The `npx tsc` route SHALL strip npx wrapper noise and summarize parseable diagnostics.

#### Scenario: wrapper-noise suppression
- **WHEN** buffered stdout includes npx wrapper noise
- **THEN** wrapper lines are removed before parsing.

#### Scenario: grouped diagnostic summary
- **WHEN** TypeScript diagnostics match `file(line,col): severity TSxxxx: message`
- **THEN** output is grouped by file with line/column entries.

#### Scenario: parse fallback
- **WHEN** output does not match parseable diagnostic lines
- **THEN** stripped raw output is flushed unchanged.
