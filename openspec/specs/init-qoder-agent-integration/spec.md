# init-qoder-agent-integration Specification

## Purpose
Define how `ccp init` detects, installs, verifies, and uninstalls the Qoder integration.

## Requirements
### Requirement: Detect Qoder Projects by `.qoder`

`ccp init` MUST detect Qoder projects using the repo-local `.qoder` directory heuristic.

#### Scenario: detect Qoder from project marker
- **WHEN** the current repository contains a `.qoder` directory
- **THEN** `qoder` is included in detected init candidates

### Requirement: Manage Qoder Guidance in Repo-Root `AGENTS.md`

`ccp init --tools qoder` MUST install CCP-managed guidance into repo-root `AGENTS.md`.

#### Scenario: install managed CCP block into repo AGENTS file
- **WHEN** the user runs `ccp init --tools qoder`
- **THEN** CCP resolves `AGENTS.md` in the current repository root as the managed Qoder target
- **AND** installs a CCP-managed block with begin and end markers
- **AND** preserves unrelated user-authored content outside the managed block

#### Scenario: rerun remains idempotent
- **GIVEN** repo-root `AGENTS.md` already contains the CCP-managed block for Qoder
- **WHEN** the user reruns `ccp init --tools qoder`
- **THEN** CCP does not duplicate the managed block
- **AND** the resulting file remains semantically unchanged

### Requirement: Verify Qoder Managed Guidance

Qoder verification MUST confirm that repo-root `AGENTS.md` still contains the CCP-managed block.

#### Scenario: verify repo AGENTS managed block
- **WHEN** repo-root `AGENTS.md` contains the CCP-managed block
- **THEN** Qoder verification succeeds

### Requirement: Remove Only CCP-Managed Qoder Content

`ccp uninstall` MUST remove only the CCP-managed Qoder block from repo-root `AGENTS.md`.

#### Scenario: uninstall preserves unrelated AGENTS content
- **GIVEN** repo-root `AGENTS.md` contains user-authored content and the CCP-managed block
- **WHEN** the user uninstalls Qoder integration
- **THEN** CCP removes only the managed block
- **AND** preserves the user-authored content

#### Scenario: uninstall removes empty AGENTS file
- **GIVEN** repo-root `AGENTS.md` contains only the CCP-managed block
- **WHEN** the user uninstalls Qoder integration
- **THEN** CCP removes the file entirely
