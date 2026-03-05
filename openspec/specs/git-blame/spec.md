## Purpose
Define `git blame` filter behavior.

## Requirements

### Requirement: Blame Passthrough Safety
`git blame` SHALL use safe behavior with targeted compaction for `--line-porcelain`.

#### Scenario: Prepare stage
- **WHEN** preparing `git blame --line-porcelain`
- **THEN** compaction is enabled.
- **WHEN** preparing other `git blame` shapes
- **THEN** `ForcePassthrough=true`.

#### Scenario: Runtime processing for line porcelain
- **WHEN** processing successful `git blame --line-porcelain` output
- **THEN** emit compact per-line records with file/line, author, committer, normalized timestamps, short hash, and content.
- **AND** emit a short summary header.

#### Scenario: Runtime passthrough on failure
- **WHEN** `git blame` exits non-zero
- **THEN** output is passed through unchanged.
