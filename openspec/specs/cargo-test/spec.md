## Purpose
Define `cargo test` subfilter prepare and runtime compaction behavior.

## Requirements

### Requirement: Cargo Test Compacting
The `cargo test` subcommand SHALL emit section-aware summary plus prioritized failure details.

#### Scenario: prepare passthrough-neutral
- **WHEN** `cargo test` prepare runs
- **THEN** args are preserved unchanged.

#### Scenario: event handling and flush timing
- **WHEN** line/tick events arrive
- **THEN** output is collected.
- **AND** compaction/flush occurs at EOF.
- **AND** exit events do not emit additional output.

#### Scenario: empty EOF buffer is ignored
- **WHEN** EOF is processed with empty buffered output
- **THEN** no compact summary is emitted.

#### Scenario: section-aware totals
- **WHEN** output includes unit/integration/doc-test section markers and result lines
- **THEN** compact output includes global totals and per-section summaries.

#### Scenario: failure-detail prioritization
- **WHEN** failures are present
- **THEN** failure details are prioritized (`error`/`panic`/`failed`) and collapsed into concise entries where possible.
- **AND** details are bounded with `... +N more` truncation.

#### Scenario: bounded failure output
- **WHEN** failure detail volume exceeds display budget
- **THEN** details are truncated deterministically after 20 lines with a `+N more` indicator.

#### Scenario: package-only failure suppression
- **WHEN** only package-level rerun guidance is present (for example `error: test failed, to rerun pass ...`) with no section counts
- **THEN** compact output is dropped for that stream.

#### Scenario: doc-test line reference preservation
- **WHEN** doc-test failures include references like `src/lib.rs - (line N)`
- **THEN** those references remain visible in compact output.

#### Scenario: low-confidence fallback
- **WHEN** test output is unrecognized or low-confidence (including binary/NUL payloads)
- **THEN** raw buffered output is flushed unchanged.
