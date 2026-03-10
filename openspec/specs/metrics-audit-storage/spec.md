# metrics-audit-storage Specification

## Purpose
Define persistent execution-metrics storage and query contracts that power `ccp gain` and `ccp history`, including deterministic token-estimate presentation in text and machine-readable outputs.
## Requirements
### Requirement: Local Execution Metrics Persistence
The system SHALL persist per-command execution metrics in a local embedded database for gain analytics.

#### Scenario: Persist proxied execution record
- **WHEN** a wrapped execution command completes through semantic proxy filtering
- **THEN** the system writes one metrics record including timestamp, command text, tool classification, dispatch key, raw bytes, kept bytes, exit code, duration, and passthrough status.

#### Scenario: Persist passthrough execution record
- **WHEN** a wrapped execution command completes in passthrough mode within proxy execution flow
- **THEN** the system writes one metrics record including the same core fields
- **AND** marks the record as passthrough
- **AND** retains the canonical tool classification when the execution plan recognized the command through a registered tool contract.

#### Scenario: Persisted command text is bounded
- **WHEN** a command text exceeds the configured storage bound
- **THEN** the persisted `command_text` is deterministically truncated to 1024 characters before write
- **AND** truncation uses `prefix + "..."` where prefix length is 1021 characters
- **AND** shorter command text values are stored unchanged.

### Requirement: Queryable Metrics Views
The metrics store SHALL support query operations where dataset selection is independent from output representation.

#### Scenario: Summary query returns per-command breakdown and total
- **WHEN** `ccp gain` requests a summary query
- **THEN** the store returns rows grouped by command for the selected dataset
- **AND** returns an additional total aggregate across all grouped rows.

#### Scenario: Summary human output can derive postable totals and winners
- **WHEN** `ccp gain` renders the default human-readable summary
- **THEN** the available query views support deriving overall totals for the selected dataset
- **AND** support identifying the strongest gains within the selected selection window.

#### Scenario: Summary human output can derive detractor explanation
- **WHEN** `ccp gain` renders the default human-readable summary
- **THEN** the available query views support identifying high-volume near-zero-savings tools, passthrough-heavy mixes, or already-compact command mixes within the selected dataset
- **AND** do not require new persisted metrics fields solely to explain detractors.

#### Scenario: Summary table output returns per-tool breakdown
- **WHEN** `ccp gain --table` renders summary output
- **THEN** rows are aggregated by `tool` (not command)
- **AND** output includes `COUNT`, `NATIVE`, `PROXIED`, and `SAVINGS` columns
- **AND** `NATIVE` and `PROXIED` are estimated token counts derived from raw and kept bytes.

#### Scenario: Summary table ordering is deterministic
- **WHEN** `ccp gain --table` renders multiple summary rows
- **THEN** rows are ordered by `COUNT` descending
- **AND** ties are ordered by `NATIVE` descending
- **AND** remaining ties are ordered by tool name ascending.

#### Scenario: Summary table output excludes passthrough top-list section
- **WHEN** `ccp gain --table` renders summary output
- **THEN** it does not append a separate "missed opportunities" passthrough command list
- **AND** the report ends after the tool summary table and `TOTAL` row.

#### Scenario: Filtered history query
- **WHEN** `ccp history` requests history with filters for `since`, `tool`, or `failed`
- **THEN** the store returns only matching records in deterministic timestamp order.

#### Scenario: recognized passthrough rows remain filterable by tool
- **WHEN** `ccp history` filters records by tool
- **AND** matching records include passthrough executions recognized by a registered tool contract
- **THEN** those records remain queryable under their canonical tool classification
- **AND** are not rewritten to `unknown`.

#### Scenario: Recent-window summary selection is supported
- **WHEN** `ccp gain` requests human-readable summaries for `--period day|week|month`
- **THEN** the available query views support selecting only the last `24h`, `7d`, or `30d` of matching records
- **AND** preserve the same filter semantics used by non-period summary selection.

#### Scenario: Recent-window summary can derive standout day signals
- **WHEN** `ccp gain --period week` renders the last-seven-days summary
- **THEN** the available query views support identifying the busiest day in that window by command count
- **AND** support identifying the best day in that window by savings efficiency
- **AND** support a recent-trend comparison within the same seven-day window.

#### Scenario: Period aggregation query remains available for table and export views
- **WHEN** `ccp gain` requests period aggregation for `day`, `week`, or `month` in detailed or machine-readable output modes
- **THEN** the store returns aggregated buckets with total commands, raw bytes, kept bytes, estimated input tokens, estimated output tokens, estimated saved tokens, and savings percentage.

#### Scenario: Weekly bucket semantics are stable
- **WHEN** a period query uses `week`
- **THEN** week buckets are Monday-based
- **AND** each bucket includes explicit start and end date boundaries.

#### Scenario: Selection is reusable across representations
- **WHEN** the same selection parameters are used with different output formats for the same dataset (`summary`, `period`, or `history`)
- **THEN** the underlying selected records are equivalent
- **AND** only output representation changes.

### Requirement: Estimated Token Derivation
Estimated token metrics SHALL be derived from byte counts using a deterministic 4-bytes-per-token heuristic.

#### Scenario: Estimate tokens from bytes
- **WHEN** gain summary, history, or period outputs include token metrics
- **THEN** estimated token values are computed from byte counts using an integer-equivalent `ceil(bytes/4)` formula.

#### Scenario: Estimated metrics are labeled
- **WHEN** gain output includes token estimates in machine-readable or human-readable formats
- **THEN** the output labels those values as estimated using the 4-bytes-per-token heuristic.

### Requirement: Unknown Classification Is Reserved For Unrecognized Commands
Metrics persistence and query views SHALL reserve `unknown` tool classification for commands that CCP could not classify through a registered tool contract or intentionally neutral shell-fallback plan.

#### Scenario: true unknown command persists as unknown
- **WHEN** a command executes without matching any registered tool contract
- **THEN** the persisted metrics record uses `tool = "unknown"` or equivalent neutral classification.

#### Scenario: recognized passthrough does not persist as unknown
- **WHEN** a command executes in passthrough mode after matching a registered tool contract
- **THEN** the persisted metrics record does not rewrite that tool classification to `unknown`.
