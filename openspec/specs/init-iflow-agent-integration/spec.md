# init-iflow-agent-integration Specification

## Purpose
Define repository-scoped iFlow detection with home-scoped managed integration for `ccp init` and `ccp uninstall`.

## Requirements
### Requirement: Detect iFlow Projects via `.iflow`

`ccp init` MUST treat `.iflow` as the initial repository-scoped detection heuristic for iFlow projects.

#### Scenario: detect iFlow project from known directory
- **WHEN** the current repository contains a `.iflow` directory
- **THEN** `ccp init` includes `iflow` in the detected tool candidates when `--tools` is omitted

### Requirement: Install CCP Guidance Into Repo-Root `IFLOW.md`

The iFlow integration MUST install CCP-managed guidance into the home-scoped `IFLOW.md` target.

#### Scenario: install managed iflow block into home file
- **WHEN** the user runs `ccp init --tools iflow`
- **THEN** CCP resolves `~/.iflow/IFLOW.md` as the canonical iFlow target
- **AND** upserts its managed block into that file
- **AND** preserves unrelated user content

### Requirement: Reapply iFlow Guidance Idempotently

iFlow installs MUST remain idempotent across repeated `ccp init` runs against the home-scoped target.

#### Scenario: rerun does not duplicate managed block
- **GIVEN** `~/.iflow/IFLOW.md` already contains the CCP-managed iFlow guidance
- **WHEN** the user reruns `ccp init --tools iflow`
- **THEN** CCP does not duplicate the managed block
- **AND** the resulting file remains semantically unchanged

### Requirement: Remove Only CCP-Managed iFlow Content

`ccp uninstall --tools iflow` MUST remove only CCP-managed iFlow content from `~/.iflow/IFLOW.md`.

#### Scenario: uninstall preserves unrelated IFLOW content
- **GIVEN** `~/.iflow/IFLOW.md` contains both the CCP-managed iFlow block and unrelated user content
- **WHEN** the user runs `ccp uninstall --tools iflow`
- **THEN** CCP removes only the managed iFlow block
- **AND** preserves the unrelated user content

#### Scenario: uninstall removes IFLOW file when block is sole content
- **GIVEN** `~/.iflow/IFLOW.md` contains only the CCP-managed iFlow block
- **WHEN** the user runs `ccp uninstall --tools iflow`
- **THEN** CCP removes the managed block
- **AND** deletes `~/.iflow/IFLOW.md`
