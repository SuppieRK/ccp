## Purpose
Define `git pull` filter behavior.

## Requirements

### Requirement: Pull Prepare Passthrough
`git pull` SHALL preserve command arguments during prepare.

#### Scenario: Args unchanged
- **WHEN** prepare runs for `git pull`
- **THEN** normalized args are identical to input args.

### Requirement: Pull Exit-Aware Summary
`git pull` SHALL summarize success and preserve raw failure diagnostics.

#### Scenario: Pull failure
- **WHEN** `git pull` exits non-zero
- **AND** buffered output is non-empty
- **THEN** buffered output is flushed unchanged.

#### Scenario: Pull failure with empty buffer
- **WHEN** `git pull` exits non-zero
- **AND** buffered output is empty
- **THEN** no output is emitted.

#### Scenario: Up-to-date pull
- **WHEN** successful output contains `already up to date` or `already up-to-date`
- **THEN** output is `Up-to-date`.

#### Scenario: Generic pull success
- **WHEN** pull exits zero and is not up-to-date
- **THEN** output is `OK`.
