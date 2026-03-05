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
- **AND** marks the record as passthrough.

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

#### Scenario: Summary text output returns per-tool breakdown
- **WHEN** `ccp gain --format text` renders summary output
- **THEN** rows are aggregated by `tool` (not command)
- **AND** output includes `COUNT`, `NATIVE`, `PROXIED`, and `SAVINGS` columns
- **AND** `NATIVE` and `PROXIED` are estimated token counts derived from raw and kept bytes.

#### Scenario: Summary text ordering is deterministic
- **WHEN** `ccp gain --format text` renders multiple summary rows
- **THEN** rows are ordered by `COUNT` descending
- **AND** ties are ordered by `NATIVE` descending
- **AND** remaining ties are ordered by tool name ascending.

#### Scenario: Summary text output excludes passthrough top-list section
- **WHEN** `ccp gain --format text` renders summary output
- **THEN** it does not append a separate "missed opportunities" passthrough command list
- **AND** the report ends after the tool summary table and `TOTAL` row.

#### Scenario: Filtered history query
- **WHEN** `ccp history` requests history with filters for `since`, `tool`, or `failed`
- **THEN** the store returns only matching records in deterministic timestamp order.

#### Scenario: Period aggregation query
- **WHEN** `ccp gain` requests period aggregation for `day`, `week`, or `month`
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
