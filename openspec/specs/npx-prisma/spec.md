## Purpose
Define `npx prisma` subfilter prepare behavior and runtime success/failure handling.

## Requirements

### Requirement: prisma Prepare Passthrough
The `npx prisma` route SHALL preserve input args in prepare.

#### Scenario: prepare preserves args
- **WHEN** prepare runs for `npx prisma`
- **THEN** normalized args are identical to input args.

### Requirement: prisma Runtime Stream Handling
The `npx prisma` route SHALL preserve stderr immediacy and flush stdout decisions at exit.

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

### Requirement: prisma Success Summaries
The `npx prisma` route SHALL summarize recognized successful validate/format output and preserve failures.

#### Scenario: wrapper-noise suppression
- **WHEN** buffered stdout contains npx wrapper lines
- **THEN** wrapper lines are removed.

#### Scenario: validate success summary
- **WHEN** successful output contains schema-valid markers
- **THEN** output is `prisma validate: ok`.

#### Scenario: format success summary
- **WHEN** successful output contains `Formatted <path> in <ms>`
- **THEN** output is `prisma format: ok <path>`.

#### Scenario: failure passthrough
- **WHEN** process exits non-zero
- **THEN** stripped raw output is flushed unchanged.

#### Scenario: unknown success output fallback
- **WHEN** process exits zero and no known success summary pattern is recognized
- **THEN** stripped raw output is flushed unchanged.
