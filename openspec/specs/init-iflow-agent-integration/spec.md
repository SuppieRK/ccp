# init-iflow-agent-integration Specification

## Purpose
Define repository-scoped iFlow integration for `ccp init` and `ccp uninstall`.

## Requirements
### Requirement: Detect iFlow Projects via `.iflow`

`ccp init` MUST treat `.iflow` as the initial repository-scoped detection heuristic for iFlow projects.

#### Scenario: detect iFlow project from known directory
- **WHEN** the current repository contains a `.iflow` directory
- **THEN** `ccp init` includes `iflow` in the detected tool candidates when `--tools` is omitted

### Requirement: Install CCP Guidance Into Repo-Root `IFLOW.md`

The iFlow integration MUST install CCP-managed guidance into the repository's `IFLOW.md`.

#### Scenario: install managed iFlow block into existing IFLOW file
- **GIVEN** the repository contains a `.iflow` directory
- **AND** repo-root `IFLOW.md` already exists with non-CCP content
- **WHEN** the user runs `ccp init --tools iflow`
- **THEN** CCP upserts its managed block into repo-root `IFLOW.md`
- **AND** preserves unrelated user content

#### Scenario: install managed iFlow block into new IFLOW file
- **GIVEN** the repository contains a `.iflow` directory
- **AND** repo-root `IFLOW.md` does not exist
- **WHEN** the user runs `ccp init --tools iflow`
- **THEN** CCP creates repo-root `IFLOW.md`
- **AND** writes the CCP-managed iFlow guidance block into that file

### Requirement: Reapply iFlow Guidance Idempotently

iFlow installs MUST remain idempotent across repeated `ccp init` runs.

#### Scenario: rerun does not duplicate managed block
- **GIVEN** repo-root `IFLOW.md` already contains the CCP-managed iFlow guidance
- **WHEN** the user reruns `ccp init --tools iflow`
- **THEN** CCP does not duplicate the managed block
- **AND** the resulting file remains semantically unchanged

### Requirement: Remove Only CCP-Managed iFlow Content

`ccp uninstall --tools iflow` MUST remove only CCP-managed iFlow content from repo-root `IFLOW.md`.

#### Scenario: uninstall preserves unrelated IFLOW content
- **GIVEN** repo-root `IFLOW.md` contains both the CCP-managed iFlow block and unrelated user content
- **WHEN** the user runs `ccp uninstall --tools iflow`
- **THEN** CCP removes only the managed iFlow block
- **AND** preserves the unrelated user content

#### Scenario: uninstall removes IFLOW file when block is sole content
- **GIVEN** repo-root `IFLOW.md` contains only the CCP-managed iFlow block
- **WHEN** the user runs `ccp uninstall --tools iflow`
- **THEN** CCP removes the managed block
- **AND** deletes repo-root `IFLOW.md`
