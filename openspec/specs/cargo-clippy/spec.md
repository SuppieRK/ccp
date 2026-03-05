## Purpose
Define `cargo clippy` subfilter prepare and runtime compaction behavior.

## Requirements

### Requirement: Cargo Clippy Compacting
The `cargo clippy` subcommand SHALL summarize findings by lint rule with bounded examples.

#### Scenario: prepare passthrough-neutral
- **WHEN** `cargo clippy` prepare runs
- **THEN** args are preserved unchanged.

#### Scenario: shared stream context
- **WHEN** runtime context key is computed
- **THEN** stdout/stderr use shared context key to keep split diagnostics together.

#### Scenario: event handling and flush timing
- **WHEN** line/tick/EOF events arrive
- **THEN** output is collected.
- **AND** compaction/flush occurs on exit to avoid split stdout/stderr diagnostics.

#### Scenario: empty output is ignored
- **WHEN** exit is processed with empty buffered output
- **THEN** no compact summary is emitted.

#### Scenario: no-issue summary
- **WHEN** clippy run reports no actionable findings
- **THEN** compact output is `cargo clippy: ok`.

#### Scenario: grouped lint summary
- **WHEN** clippy findings are parseable
- **THEN** output includes total findings, lint-rule counts, first location (if available), and bounded examples.
- **AND** lint rules normalize hyphenated names to underscore form.

#### Scenario: issue retention and example cap
- **WHEN** clippy reports warnings/errors
- **THEN** issue diagnostics remain visible with rule/context identity while low-signal lines are compacted.
- **AND** each lint-rule group emits at most 3 representative examples before `+N more`.

#### Scenario: low-confidence fallback
- **WHEN** compactor cannot parse with confidence (including binary/NUL payloads)
- **THEN** raw buffered output is flushed unchanged.
