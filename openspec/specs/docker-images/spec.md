## Purpose
Define `docker images` routing constraints and runtime compaction behavior.

## Requirements

### Requirement: Docker Images Parent Dispatch Safety
The `docker` parent route for `images` SHALL normalize only default shape and avoid rewriting explicit structured output.

#### Scenario: explicit format passthrough
- **WHEN** `docker images` includes `--format` (including `--format=...`)
- **THEN** parent returns ambiguous passthrough with structured-output reason.

#### Scenario: zero-arg normalization
- **WHEN** `docker images` is invoked without additional args
- **THEN** args are normalized to `images --format {{.Repository}}:{{.Tag}}\t{{.Size}}`.
- **AND** dispatch key is `docker images`.

#### Scenario: non-empty args dispatch
- **WHEN** `docker images` has args and no explicit `--format`
- **THEN** args are preserved and dispatch key is `docker images`.

### Requirement: Docker Images Runtime Handling
The `docker images` subfilter SHALL keep stderr immediate and compact collected stdout on EOF or exit.

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

### Requirement: Docker Images Compaction Outcomes
The `docker images` compactor SHALL summarize image count and size for supported shapes, with bounded rows and fallback safety.

#### Scenario: supported parse modes
- **WHEN** stdout is parseable as structured rows (`repo:tag\tsize`) or canonical table headers (`REPOSITORY`, `TAG`, `SIZE`)
- **THEN** compact output includes total image count and total size.

#### Scenario: bounded row rendering
- **WHEN** image rows exceed render budget
- **THEN** output includes first rows and `... +N more`.

#### Scenario: parse-confidence fallback
- **WHEN** output is not parseable in supported shapes
- **THEN** raw output is flushed unchanged.
