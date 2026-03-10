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

`ccp init --tools factory` MUST install CCP-managed guidance into Factory's home-scoped `AGENTS.md`.

#### Scenario: install managed block into home factory agents file
- **WHEN** the user runs `ccp init --tools factory`
- **THEN** CCP resolves `~/.factory/AGENTS.md` as the managed Factory target
- **AND** installs a CCP-managed block with begin and end markers
- **AND** preserves unrelated user-authored content outside the managed block

#### Scenario: rerun remains idempotent
- **GIVEN** `~/.factory/AGENTS.md` already contains the CCP-managed block for Factory
- **WHEN** the user reruns `ccp init --tools factory`
- **THEN** CCP does not duplicate the managed block
- **AND** the resulting file remains semantically unchanged

### Requirement: Verify Factory Managed Guidance

Factory verification MUST confirm that `~/.factory/AGENTS.md` still contains the CCP-managed block.

#### Scenario: verify home factory managed block
- **WHEN** `~/.factory/AGENTS.md` contains the CCP-managed block
- **THEN** Factory verification succeeds

### Requirement: Remove Only CCP-Managed Factory Content

`ccp uninstall` MUST remove only the CCP-managed Factory block from `~/.factory/AGENTS.md`.

#### Scenario: uninstall preserves unrelated home agents content
- **GIVEN** `~/.factory/AGENTS.md` contains user-authored content and the CCP-managed block
- **WHEN** the user uninstalls Factory integration
- **THEN** CCP removes only the managed block
- **AND** preserves the user-authored content

#### Scenario: uninstall removes empty home agents file
- **GIVEN** `~/.factory/AGENTS.md` contains only the CCP-managed block
- **WHEN** the user uninstalls Factory integration
- **THEN** CCP removes the file entirely
