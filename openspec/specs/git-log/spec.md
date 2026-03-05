## Purpose
Define `git log` filter behavior.

## Requirements

### Requirement: Log Preparation Defaults
`git log` SHALL apply compact defaults unless user flags already provide explicit format/limit/merge intent.

#### Scenario: Missing format
- **WHEN** no explicit format flag is supplied
- **THEN** `--pretty=format:%h %aI %an <%ae> | %s` is added.

#### Scenario: Missing limit
- **WHEN** no numeric short limit flag is supplied
- **THEN** `-10` is added.

#### Scenario: Merge-noise reduction
- **WHEN** merges are not explicitly requested
- **THEN** `--no-merges` is added.

### Requirement: Log Runtime Handling
`git log` SHALL pass stderr immediately and compact stdout on EOF.

#### Scenario: Stderr line
- **WHEN** stderr line is received
- **THEN** it is emitted immediately.

#### Scenario: Long line truncation
- **WHEN** compacting stdout lines
- **THEN** lines longer than 120 chars are truncated.
