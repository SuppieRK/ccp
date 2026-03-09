# init-factory-agent-integration Specification

## Purpose
Define how `ccp init` detects, installs, verifies, and uninstalls the Factory integration.

## Requirements
### Requirement: Detect Factory Projects by `.factory`

`ccp init` MUST detect Factory projects using the repo-local `.factory` directory heuristic.

#### Scenario: detect Factory from project marker
- **WHEN** the current repository contains a `.factory` directory
- **THEN** `factory` is included in detected init candidates

### Requirement: Manage Factory Guidance in Repo-Root `AGENTS.md`

`ccp init --tools factory` MUST install CCP-managed guidance into repo-root `AGENTS.md`.

#### Scenario: install managed CCP block into repo AGENTS file
- **WHEN** the user runs `ccp init --tools factory`
- **THEN** CCP resolves `AGENTS.md` in the current repository root as the managed Factory target
- **AND** installs a CCP-managed block with begin and end markers
- **AND** preserves unrelated user-authored content outside the managed block

#### Scenario: rerun remains idempotent
- **GIVEN** repo-root `AGENTS.md` already contains the CCP-managed block for Factory
- **WHEN** the user reruns `ccp init --tools factory`
- **THEN** CCP does not duplicate the managed block
- **AND** the resulting file remains semantically unchanged

### Requirement: Verify Factory Managed Guidance

Factory verification MUST confirm that repo-root `AGENTS.md` still contains the CCP-managed block.

#### Scenario: verify repo AGENTS managed block
- **WHEN** repo-root `AGENTS.md` contains the CCP-managed block
- **THEN** Factory verification succeeds

### Requirement: Remove Only CCP-Managed Factory Content

`ccp uninstall` MUST remove only the CCP-managed Factory block from repo-root `AGENTS.md`.

#### Scenario: uninstall preserves unrelated AGENTS content
- **GIVEN** repo-root `AGENTS.md` contains user-authored content and the CCP-managed block
- **WHEN** the user uninstalls Factory integration
- **THEN** CCP removes only the managed block
- **AND** preserves the user-authored content

#### Scenario: uninstall removes empty AGENTS file
- **GIVEN** repo-root `AGENTS.md` contains only the CCP-managed block
- **WHEN** the user uninstalls Factory integration
- **THEN** CCP removes the file entirely
