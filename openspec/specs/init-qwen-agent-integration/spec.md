# init-qwen-agent-integration Specification

## Purpose
Define how `ccp init` detects, installs, verifies, and uninstalls the Qwen Code integration.

## Requirements
### Requirement: Detect Qwen Projects by `.qwen`

`ccp init` MUST detect Qwen Code projects using the repo-local `.qwen` directory heuristic.

#### Scenario: detect Qwen from project marker
- **WHEN** the current repository contains a `.qwen` directory
- **THEN** `qwen` is included in detected init candidates

### Requirement: Manage Qwen Guidance in Repo-Root `AGENTS.md`

`ccp init --tools qwen` MUST install CCP-managed guidance into repo-root `AGENTS.md`.

#### Scenario: install managed CCP block into repo AGENTS file
- **WHEN** the user runs `ccp init --tools qwen`
- **THEN** CCP resolves `AGENTS.md` in the current repository root as the managed Qwen target
- **AND** installs a CCP-managed block with begin and end markers
- **AND** preserves unrelated user-authored content outside the managed block

#### Scenario: rerun remains idempotent
- **GIVEN** repo-root `AGENTS.md` already contains the CCP-managed block for Qwen
- **WHEN** the user reruns `ccp init --tools qwen`
- **THEN** CCP does not duplicate the managed block
- **AND** the resulting file remains semantically unchanged

### Requirement: Verify Qwen Managed Guidance

Qwen verification MUST confirm that repo-root `AGENTS.md` still contains the CCP-managed block.

#### Scenario: verify repo AGENTS managed block
- **WHEN** repo-root `AGENTS.md` contains the CCP-managed block
- **THEN** Qwen verification succeeds

### Requirement: Remove Only CCP-Managed Qwen Content

`ccp uninstall` MUST remove only the CCP-managed Qwen block from repo-root `AGENTS.md`.

#### Scenario: uninstall preserves unrelated AGENTS content
- **GIVEN** repo-root `AGENTS.md` contains user-authored content and the CCP-managed block
- **WHEN** the user uninstalls Qwen integration
- **THEN** CCP removes only the managed block
- **AND** preserves the user-authored content

#### Scenario: uninstall removes empty AGENTS file
- **GIVEN** repo-root `AGENTS.md` contains only the CCP-managed block
- **WHEN** the user uninstalls Qwen integration
- **THEN** CCP removes the file entirely
