# init-pi-agent-integration Specification

## Purpose
Define repository-scoped Pi integration for `ccp init` and `ccp uninstall`.

## Requirements
### Requirement: Detect Pi Projects via `.pi`

`ccp init` MUST treat `.pi` as the initial repository-scoped detection heuristic for Pi projects.

#### Scenario: detect Pi project from known directory
- **WHEN** the current repository contains a `.pi` directory
- **THEN** `ccp init` includes `pi` in the detected tool candidates when `--tools` is omitted

### Requirement: Install CCP Guidance Into Repo-Root `AGENTS.md`

The Pi integration MUST install CCP-managed guidance into the repository's `AGENTS.md`.

#### Scenario: install managed Pi block into existing agents file
- **GIVEN** the repository contains a `.pi` directory
- **AND** repo-root `AGENTS.md` already exists with non-CCP content
- **WHEN** the user runs `ccp init --tools pi`
- **THEN** CCP upserts its managed block into repo-root `AGENTS.md`
- **AND** preserves unrelated user content

#### Scenario: install managed Pi block into new agents file
- **GIVEN** the repository contains a `.pi` directory
- **AND** repo-root `AGENTS.md` does not exist
- **WHEN** the user runs `ccp init --tools pi`
- **THEN** CCP creates repo-root `AGENTS.md`
- **AND** writes the CCP-managed Pi guidance block into that file

### Requirement: Reapply Pi Guidance Idempotently

Pi installs MUST remain idempotent across repeated `ccp init` runs.

#### Scenario: rerun does not duplicate managed block
- **GIVEN** repo-root `AGENTS.md` already contains the CCP-managed Pi guidance
- **WHEN** the user reruns `ccp init --tools pi`
- **THEN** CCP does not duplicate the managed block
- **AND** the resulting file remains semantically unchanged

### Requirement: Remove Only CCP-Managed Pi Content

`ccp uninstall --tools pi` MUST remove only CCP-managed Pi content from repo-root `AGENTS.md`.

#### Scenario: uninstall preserves unrelated agents content
- **GIVEN** repo-root `AGENTS.md` contains both the CCP-managed Pi block and unrelated user content
- **WHEN** the user runs `ccp uninstall --tools pi`
- **THEN** CCP removes only the managed Pi block
- **AND** preserves the unrelated user content

#### Scenario: uninstall removes agents file when block is sole content
- **GIVEN** repo-root `AGENTS.md` contains only the CCP-managed Pi block
- **WHEN** the user runs `ccp uninstall --tools pi`
- **THEN** CCP removes the managed block
- **AND** deletes repo-root `AGENTS.md`
