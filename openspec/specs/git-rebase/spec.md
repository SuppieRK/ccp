## Purpose
Define `git rebase` filter behavior.

## Requirements

### Requirement: Rebase Prepare Passthrough
`git rebase` SHALL preserve command arguments during prepare.

#### Scenario: Args unchanged
- **WHEN** prepare runs for `git rebase`
- **THEN** normalized args are identical to input args.

### Requirement: Rebase Exit-Aware Confirmation
`git rebase` SHALL emit compact success and preserve failure diagnostics.

#### Scenario: Rebase success
- **WHEN** `git rebase` exits zero
- **THEN** output is `ok rebase`.

#### Scenario: Rebase failure
- **WHEN** `git rebase` exits non-zero
- **AND** buffered diagnostics are non-empty
- **THEN** raw buffered diagnostics are emitted unchanged.

#### Scenario: Rebase failure with empty buffer
- **WHEN** `git rebase` exits non-zero
- **AND** buffered diagnostics are empty
- **THEN** no output is emitted.
