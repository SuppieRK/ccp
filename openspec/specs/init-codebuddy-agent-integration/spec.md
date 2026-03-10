# init-codebuddy-agent-integration Specification

## Purpose
Define how `ccp init` detects, installs, verifies, and uninstalls the CodeBuddy integration.

## Requirements
### Requirement: Detect CodeBuddy Projects by `.codebuddy`

`ccp init` MUST detect CodeBuddy projects using the repo-local `.codebuddy` directory heuristic.

#### Scenario: detect CodeBuddy from project marker
- **WHEN** the current repository contains a `.codebuddy` directory
- **THEN** `codebuddy` is included in detected init candidates

### Requirement: CodeBuddy Uses User-Scoped Settings Hook Integration
CodeBuddy init integration SHALL install CCP through CodeBuddy's user-scoped `settings.json` hook system rather than through repo-root memory files.

#### Scenario: install user-scoped codebuddy settings hook
- **WHEN** the user runs `ccp init --tools codebuddy`
- **THEN** CCP installs a managed hook script under `~/.codebuddy/hooks/`
- **AND** CCP installs or updates `~/.codebuddy/settings.json`
- **AND** the managed integration uses `PreToolUse` with `updatedInput` command interception to route shell execution through `ccp`

#### Scenario: hook runtime uses only bash builtins and ccp
- **WHEN** the managed CodeBuddy hook executes
- **THEN** its runtime behavior does not depend on helper commands other than bash builtins and `ccp`

#### Scenario: exit paths leave troubleshooting markers
- **WHEN** the managed CodeBuddy hook exits with status `0`
- **THEN** it appends a deterministic reason marker to a log file under the system tmp directory
- **AND** each early-return branch uses a distinct marker

### Requirement: CodeBuddy Uninstall Removes Managed Settings Artifacts
CodeBuddy uninstall integration SHALL remove only the CCP-managed settings and hook artifacts.

#### Scenario: uninstall preserves unrelated settings
- **WHEN** the user uninstalls CodeBuddy integration
- **THEN** CCP removes only the CCP-managed settings entry and hook artifacts
- **AND** preserves unrelated CodeBuddy settings, hooks, and memory files
