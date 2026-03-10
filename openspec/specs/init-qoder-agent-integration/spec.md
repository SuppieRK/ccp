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

`ccp init --tools qoder` MUST install CCP-managed guidance into the home-scoped Qoder `AGENTS.md`.

#### Scenario: install managed qoder block into home agents file
- **WHEN** the user runs `ccp init --tools qoder`
- **THEN** CCP resolves `~/.qoder/AGENTS.md` as the managed Qoder target
- **AND** installs a CCP-managed block with begin and end markers
- **AND** preserves unrelated user-authored content outside the managed block

#### Scenario: rerun remains idempotent
- **GIVEN** `~/.qoder/AGENTS.md` already contains the CCP-managed block for Qoder
- **WHEN** the user reruns `ccp init --tools qoder`
- **THEN** CCP does not duplicate the managed block
- **AND** the resulting file remains semantically unchanged

### Requirement: Verify Qoder Managed Guidance

Qoder verification MUST confirm that `~/.qoder/AGENTS.md` still contains the CCP-managed block.

#### Scenario: verify home qoder managed block
- **WHEN** `~/.qoder/AGENTS.md` contains the CCP-managed block
- **THEN** Qoder verification succeeds

### Requirement: Remove Only CCP-Managed Qoder Content

`ccp uninstall` MUST remove only the CCP-managed Qoder block from `~/.qoder/AGENTS.md`.

#### Scenario: uninstall preserves unrelated qoder agents content
- **GIVEN** `~/.qoder/AGENTS.md` contains user-authored content and the CCP-managed block
- **WHEN** the user uninstalls Qoder integration
- **THEN** CCP removes only the managed block
- **AND** preserves the user-authored content

#### Scenario: uninstall removes empty qoder agents file
- **GIVEN** `~/.qoder/AGENTS.md` contains only the CCP-managed block
- **WHEN** the user uninstalls Qoder integration
- **THEN** CCP removes the file entirely
