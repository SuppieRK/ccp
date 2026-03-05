## Purpose
Define `npx node` subfilter prepare safety and runtime compaction behavior.

## Requirements

### Requirement: npx node Prepare Safety
The `npx node` route SHALL passthrough interactive invocation shapes and otherwise preserve args.

#### Scenario: interactive invocation passthrough
- **WHEN** node args represent interactive invocation
- **THEN** prepare returns passthrough.

#### Scenario: non-interactive prepare
- **WHEN** node args are non-interactive
- **THEN** prepare preserves args and does not force passthrough.

### Requirement: npx node Shared Runtime Context
The `npx node` route SHALL use shared node context across streams.

#### Scenario: shared node context key
- **WHEN** stdout/stderr events belong to the same command
- **THEN** context key is derived from shared `node` tool context.

### Requirement: npx Wrapper Noise Handling
The `npx node` route SHALL ignore known npx wrapper prompt/install lines on line events.

#### Scenario: wrapper-noise suppression
- **WHEN** line events match npx install/prompt boilerplate
- **THEN** those lines are ignored.

### Requirement: npx node Runtime Event Handling
The route SHALL collect tick/EOF events and compact/flush at failure line detection or exit.

#### Scenario: tick and EOF collection
- **WHEN** event type is tick or EOF
- **THEN** event is collected.

#### Scenario: warning dedupe
- **WHEN** repeated runtime warnings normalize to the same warning key
- **THEN** output keeps the first warning and emits `[+N similar warnings]`.

#### Scenario: unhandled-failure early flush
- **WHEN** an unhandled-node-failure marker is detected on line events
- **THEN** current buffered output is compacted and flushed immediately.
- **AND** if compaction cannot produce non-empty output, raw buffered output is flushed unchanged.

#### Scenario: exit with empty buffer
- **WHEN** exit is received with empty buffered output
- **THEN** no output is emitted.

#### Scenario: low-confidence fallback
- **WHEN** output confidence is low
- **THEN** raw buffered output is flushed unchanged.

#### Scenario: non-zero exit with empty compact result
- **WHEN** compaction yields empty output and exit code is non-zero
- **THEN** raw buffered output is flushed unchanged.
