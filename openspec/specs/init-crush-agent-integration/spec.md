## Purpose
Define how `ccp init` installs and manages CCP guidance for Crush CLI projects.

## Requirements
### Requirement: Detect Crush Projects via `.crush`

`ccp init` MUST treat `.crush` as the initial repository-scoped detection heuristic for Crush projects.

#### Scenario: detect Crush project from known directory
- **WHEN** the current repository contains a `.crush` directory
- **THEN** `ccp init` includes `crush` in the detected tool candidates when `--tools` is omitted

### Requirement: Install CCP Guidance Into Repo-Root `AGENTS.md`

The Crush integration MUST install CCP-managed guidance into the repository's `AGENTS.md`.

#### Scenario: install managed Crush block into existing AGENTS file
- **GIVEN** the repository contains a `.crush` directory
- **AND** repo-root `AGENTS.md` already exists with non-CCP content
- **WHEN** the user runs `ccp init --tools crush`
- **THEN** CCP upserts its managed block into repo-root `AGENTS.md`
- **AND** preserves unrelated user content

#### Scenario: install managed Crush block into new AGENTS file
- **GIVEN** the repository contains a `.crush` directory
- **AND** repo-root `AGENTS.md` does not exist
- **WHEN** the user runs `ccp init --tools crush`
- **THEN** CCP creates repo-root `AGENTS.md`
- **AND** writes the CCP-managed Crush guidance block into that file

### Requirement: Reapply Crush Guidance Idempotently

Crush installs MUST remain idempotent across repeated `ccp init` runs.

#### Scenario: rerun does not duplicate managed block
- **GIVEN** repo-root `AGENTS.md` already contains the CCP-managed Crush guidance
- **WHEN** the user reruns `ccp init --tools crush`
- **THEN** CCP does not duplicate the managed block
- **AND** the resulting file remains semantically unchanged

### Requirement: Remove Only CCP-Managed Crush Content

`ccp uninstall --tools crush` MUST remove only CCP-managed Crush content from repo-root `AGENTS.md`.

#### Scenario: uninstall preserves unrelated AGENTS content
- **GIVEN** repo-root `AGENTS.md` contains both the CCP-managed Crush block and unrelated user content
- **WHEN** the user runs `ccp uninstall --tools crush`
- **THEN** CCP removes only the managed Crush block
- **AND** preserves the unrelated user content

#### Scenario: uninstall removes AGENTS file when block is sole content
- **GIVEN** repo-root `AGENTS.md` contains only the CCP-managed Crush block
- **WHEN** the user runs `ccp uninstall --tools crush`
- **THEN** CCP removes the managed block
- **AND** deletes repo-root `AGENTS.md`
