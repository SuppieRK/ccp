## Purpose
Define `cargo build` subfilter prepare and runtime compaction behavior.

## Requirements

### Requirement: Cargo Build Compacting
The `cargo build` subcommand SHALL compact recognized build output and preserve diagnostics.

#### Scenario: prepare passthrough-neutral
- **WHEN** `cargo build` prepare runs
- **THEN** args are preserved unchanged.

#### Scenario: event handling and flush timing
- **WHEN** line/tick events arrive
- **THEN** output is collected.
- **AND** compaction/flush occurs at EOF.
- **AND** exit events do not emit additional output.

#### Scenario: empty EOF buffer is ignored
- **WHEN** EOF is processed with empty buffered output
- **THEN** no compact summary is emitted for `cargo build`.

#### Scenario: recognized compact summary
- **WHEN** output matches build/check compaction patterns
- **THEN** output is compacted to `cargo build: ok` or `cargo build: <N> diagnostics` with bounded diagnostics.

#### Scenario: compile-noise suppression
- **WHEN** output contains progress/summary noise (for example `Compiling`, `Downloading`, `Fresh`, `Finished`, or cargo summary-noise lines)
- **THEN** those lines are recognized and suppressed in compact mode.
- **AND** recognized summary-noise includes cargo error summaries such as `could not compile` / `aborting due to` and aggregate warning summaries such as `generated ... warning`.

#### Scenario: diagnostic retention and cap
- **WHEN** compile/link diagnostics are emitted
- **THEN** diagnostics remain visible in compact output with error-priority ordering.
- **AND** emitted diagnostics are capped at 20 lines with `+N more` overflow marker.

#### Scenario: low-confidence fallback
- **WHEN** output is not recognized by compactor (including binary/NUL payloads)
- **THEN** raw buffered output is flushed unchanged.
