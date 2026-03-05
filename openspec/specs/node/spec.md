## Purpose
Define `node` filter metadata, prepare safety, and runtime compaction behavior.

## Requirements

### Requirement: Node Tool Identity And Aliases
The Node filter SHALL identify as `node` and support platform alias executables.

#### Scenario: alias executables
- **WHEN** executable is `node.exe`, `./node.exe`, `node.cmd`, or `./node.cmd`
- **THEN** the Node filter contract is used.

### Requirement: Interactive Prepare Safety
Prepare SHALL passthrough interactive invocation shapes.

#### Scenario: interactive invocation passthrough
- **WHEN** invocation is interactive (for example no entry args, `-i`, `--interactive`, or flags treated as interactive by Node invocation classifier)
- **THEN** prepare forces passthrough.

#### Scenario: runtime dispatch for non-interactive invocation
- **WHEN** invocation is non-interactive
- **THEN** prepare preserves args and sets dispatch key `node|mode=runtime`.

### Requirement: Shared Context Runtime Model
Node runtime handling SHALL use shared stdout/stderr context with line-aware and exit-aware decisions.

#### Scenario: shared stream context
- **WHEN** stdout and stderr events belong to the same command
- **THEN** both streams resolve to one shared context key.

#### Scenario: tick and EOF collection
- **WHEN** event type is tick or EOF
- **THEN** event is collected.

#### Scenario: unhandled rejection line flush
- **WHEN** a line matches unhandled-failure markers (for example unhandled promise rejection)
- **THEN** buffered output is flushed immediately
- **AND** single-line fast path may flush only that line.

#### Scenario: non-failure line collection
- **WHEN** line does not match unhandled-failure markers
- **THEN** event is collected.

#### Scenario: exit with empty buffer
- **WHEN** exit is received and buffered output is empty
- **THEN** no output is emitted.

### Requirement: Node Output Compaction And Fallback
Node compaction SHALL reduce low-signal runtime boilerplate while preserving diagnostics and falling back on low confidence.

#### Scenario: warning-prefix dedupe
- **WHEN** repeated warning lines differ only by `(node:<pid>)` prefix
- **THEN** warnings are folded deterministically with retain-first behavior.
- **AND** recognized runtime progress/noise lines are removed before warning aggregation.

#### Scenario: progress artifact suppression
- **WHEN** carriage-return/spinner progress artifacts are present
- **THEN** those artifacts are suppressed.

#### Scenario: ESM/CommonJS warning folding
- **WHEN** equivalent module-mode compatibility warnings repeat
- **THEN** duplicates are folded deterministically.

#### Scenario: failure diagnostics retention
- **WHEN** output contains runtime failure diagnostics (`Error:`, stack frames, `Caused by:`)
- **THEN** diagnostics remain visible in compact output.

#### Scenario: low-confidence fallback
- **WHEN** compaction reports low confidence
- **THEN** raw buffered output is flushed unchanged.

#### Scenario: empty compact output with non-zero exit
- **WHEN** compaction yields empty output and exit code is non-zero
- **THEN** raw buffered output is flushed unchanged.
