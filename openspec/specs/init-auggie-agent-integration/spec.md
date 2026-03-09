# init-auggie-agent-integration Specification

## Purpose
Define how `ccp init` detects, installs, verifies, and uninstalls the Auggie integration.

## Requirements
### Requirement: Detect Auggie Projects by `.augment`

`ccp init` MUST detect Auggie projects using the repo-local `.augment` directory heuristic.

#### Scenario: detect Auggie from project marker
- **WHEN** the current repository contains a `.augment` directory
- **THEN** `auggie` is included in detected init candidates

### Requirement: Manage Auggie Guidance in Repo-Root `AGENTS.md`

`ccp init --tools auggie` MUST install CCP-managed guidance into repo-root `AGENTS.md`.

#### Scenario: install managed CCP block into repo AGENTS file
- **WHEN** the user runs `ccp init --tools auggie`
- **THEN** CCP resolves `AGENTS.md` in the current repository root as the managed Auggie target
- **AND** installs a CCP-managed block with begin and end markers
- **AND** preserves unrelated user-authored content outside the managed block

#### Scenario: rerun remains idempotent
- **GIVEN** repo-root `AGENTS.md` already contains the CCP-managed block for Auggie
- **WHEN** the user reruns `ccp init --tools auggie`
- **THEN** CCP does not duplicate the managed block
- **AND** the resulting file remains semantically unchanged

### Requirement: Verify Auggie Managed Guidance

Auggie verification MUST confirm that repo-root `AGENTS.md` still contains the CCP-managed block.

#### Scenario: verify repo AGENTS managed block
- **WHEN** repo-root `AGENTS.md` contains the CCP-managed block
- **THEN** Auggie verification succeeds

### Requirement: Remove Only CCP-Managed Auggie Content

`ccp uninstall` MUST remove only the CCP-managed Auggie block from repo-root `AGENTS.md`.

#### Scenario: uninstall preserves unrelated AGENTS content
- **GIVEN** repo-root `AGENTS.md` contains user-authored content and the CCP-managed block
- **WHEN** the user uninstalls Auggie integration
- **THEN** CCP removes only the managed block
- **AND** preserves the user-authored content

#### Scenario: uninstall removes empty AGENTS file
- **GIVEN** repo-root `AGENTS.md` contains only the CCP-managed block
- **WHEN** the user uninstalls Auggie integration
- **THEN** CCP removes the file entirely
