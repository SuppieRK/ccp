## Purpose
Define `docker ps` routing constraints and runtime compaction behavior.

## Requirements

### Requirement: Docker Ps Parent Dispatch Safety
The `docker` parent route for `ps` SHALL avoid rewriting user-structured output and only dispatch compactable shapes.

#### Scenario: structured output passthrough
- **WHEN** `docker ps` includes `--format` (including `--format=...`)
- **THEN** parent returns ambiguous passthrough with structured-output reason.

#### Scenario: compactable dispatch
- **WHEN** `docker ps` has no explicit `--format`
- **THEN** dispatch key is `docker ps` with args preserved.

### Requirement: Docker Ps Runtime Handling
The `docker ps` subfilter SHALL keep stderr immediate and compact collected stdout on EOF or exit.

#### Scenario: stderr passthrough
- **WHEN** stderr line events arrive
- **THEN** lines are emitted immediately unchanged.

#### Scenario: stdout collection
- **WHEN** stdout line and tick events arrive
- **THEN** events are collected.

#### Scenario: EOF or exit flush
- **WHEN** EOF or exit arrives with non-empty buffered stdout
- **THEN** compacted output is flushed.

#### Scenario: empty output parity
- **WHEN** EOF or exit arrives with empty buffered stdout
- **THEN** no output is emitted.

### Requirement: Docker Ps Compaction Outcomes
The `docker ps` compactor SHALL prioritize anomalies, fold healthy duplicates, and fall back safely.

#### Scenario: non-healthy priority
- **WHEN** non-healthy rows are present
- **THEN** anomaly rows are emitted first with explicit anomaly markers.

#### Scenario: healthy row grouping
- **WHEN** multiple healthy rows share image/status/ports characteristics
- **THEN** rows are grouped with count and joined names.

#### Scenario: bounded row rendering
- **WHEN** rendered anomaly/group lines exceed budget
- **THEN** output is truncated with `... +N more`.

#### Scenario: parse-confidence fallback
- **WHEN** output is not parseable as canonical `docker ps` table
- **THEN** raw buffered output is flushed unchanged.

#### Scenario: deterministic parse-group-render pipeline
- **WHEN** output is parseable as canonical `docker ps` table
- **THEN** compaction deterministically performs table parsing, anomaly/group aggregation, and bounded rendering.
