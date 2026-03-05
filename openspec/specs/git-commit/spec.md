## Purpose
Define `git commit` filter behavior.

## Requirements

### Requirement: Commit Exit-Aware Output
`git commit` SHALL emit compact success summaries and preserve failure diagnostics.

#### Scenario: Commit failure
- **WHEN** `git commit` exits non-zero
- **THEN** raw diagnostics are emitted.

#### Scenario: Commit hash extraction
- **WHEN** successful output contains commit header with hash
- **THEN** output includes short hash in `git commit: ok <hash>` form.

#### Scenario: Commit hash plus shortstat
- **WHEN** successful output contains commit hash and shortstat
- **THEN** output is `git commit: ok <hash> <files> files +<adds> -<dels>`.

#### Scenario: Nothing to commit
- **WHEN** successful output indicates nothing to commit
- **THEN** output is `git commit: ok (nothing to commit)`.

#### Scenario: Generic commit success
- **WHEN** successful output has no hash or shortstat
- **THEN** output is `git commit: ok`.
