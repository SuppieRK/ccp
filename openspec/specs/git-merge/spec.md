## Purpose
Define `git merge` filter behavior.

## Requirements

### Requirement: Merge Prepare Passthrough
`git merge` SHALL preserve command arguments during prepare.

#### Scenario: Args unchanged
- **WHEN** prepare runs for `git merge`
- **THEN** normalized args are identical to input args.

### Requirement: Merge Exit-Aware Confirmation
`git merge` SHALL emit compact success and preserve failure diagnostics.

#### Scenario: Merge success
- **WHEN** `git merge` exits zero
- **THEN** output is `git merge: ok`.

#### Scenario: Merge failure
- **WHEN** `git merge` exits non-zero
- **THEN** raw buffered diagnostics are emitted unchanged.
