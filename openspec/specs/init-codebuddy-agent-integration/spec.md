# init-codebuddy-agent-integration Specification

## Purpose
Define how `ccp init` detects, installs, verifies, and uninstalls the CodeBuddy integration.

## Requirements
### Requirement: Detect CodeBuddy Projects by `.codebuddy`

`ccp init` MUST detect CodeBuddy projects using the repo-local `.codebuddy` directory heuristic.

#### Scenario: detect CodeBuddy from project marker
- **WHEN** the current repository contains a `.codebuddy` directory
- **THEN** `codebuddy` is included in detected init candidates

### Requirement: Manage CodeBuddy Guidance in Repo-Root `CODEBUDDY.md`

`ccp init --tools codebuddy` MUST install CCP-managed guidance into repo-root `CODEBUDDY.md`.

#### Scenario: install managed CCP block into repo memory file
- **WHEN** the user runs `ccp init --tools codebuddy`
- **THEN** CCP resolves `CODEBUDDY.md` in the current repository root as the managed CodeBuddy target
- **AND** installs a CCP-managed block with begin and end markers
- **AND** preserves unrelated user-authored content outside the managed block

#### Scenario: rerun remains idempotent
- **GIVEN** repo-root `CODEBUDDY.md` already contains the CCP-managed block for CodeBuddy
- **WHEN** the user reruns `ccp init --tools codebuddy`
- **THEN** CCP does not duplicate the managed block
- **AND** the resulting file remains semantically unchanged

### Requirement: Verify CodeBuddy Managed Guidance

CodeBuddy verification MUST confirm that repo-root `CODEBUDDY.md` still contains the CCP-managed block.

#### Scenario: verify repo CodeBuddy memory file managed block
- **WHEN** repo-root `CODEBUDDY.md` contains the CCP-managed block
- **THEN** CodeBuddy verification succeeds

### Requirement: Remove Only CCP-Managed CodeBuddy Content

`ccp uninstall` MUST remove only the CCP-managed CodeBuddy block from repo-root `CODEBUDDY.md`.

#### Scenario: uninstall preserves unrelated CodeBuddy memory content
- **GIVEN** repo-root `CODEBUDDY.md` contains user-authored content and the CCP-managed block
- **WHEN** the user uninstalls CodeBuddy integration
- **THEN** CCP removes only the managed block
- **AND** preserves the user-authored content

#### Scenario: uninstall removes empty CodeBuddy memory file
- **GIVEN** repo-root `CODEBUDDY.md` contains only the CCP-managed block
- **WHEN** the user uninstalls CodeBuddy integration
- **THEN** CCP removes the file entirely
