## Purpose
Define `git push` filter behavior.

## Requirements

### Requirement: Push Exit-Aware Summary
`git push` SHALL summarize success and preserve raw failure diagnostics.

#### Scenario: Push failure
- **WHEN** `git push` exits non-zero
- **THEN** buffered output is flushed unchanged.

#### Scenario: Up-to-date push
- **WHEN** successful output contains `everything up-to-date`
- **THEN** output is `Up-to-date`.

#### Scenario: Generic push success
- **WHEN** push exits zero and is not up-to-date
- **THEN** output is `OK`.
