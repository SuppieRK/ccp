## Purpose
Define `git status` filter behavior.

## Requirements

### Requirement: Status Preparation
`git status` SHALL enforce porcelain output unless the user already requested porcelain mode.

#### Scenario: Porcelain default
- **WHEN** `git status` is prepared without `--porcelain`
- **THEN** `--porcelain` is appended to normalized args.

#### Scenario: Existing porcelain preserved
- **WHEN** args already contain `--porcelain` or `--porcelain=<value>`
- **THEN** args are preserved as-is.

### Requirement: Status Runtime Handling
`git status` SHALL keep stderr immediate and flush collected stdout at EOF.

#### Scenario: Stderr passthrough
- **WHEN** stderr line events arrive
- **THEN** lines are emitted immediately unchanged.

#### Scenario: Stdout flush at EOF
- **WHEN** stdout reaches EOF with non-empty buffered content
- **THEN** buffered content is flushed unchanged.

#### Scenario: Empty stdout
- **WHEN** buffered stdout is empty at EOF
- **THEN** no output is emitted.
