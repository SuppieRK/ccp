# init-cline-agent-integration Specification

## Purpose
Define the managed Cline integration installed and removed by `ccp init` and `ccp uninstall`.

## Requirements
### Requirement: Cline Detection vs Install Scope
Cline init integration SHALL detect from repository scope and install to a home-scoped hooks target.

#### Scenario: repository detection with home-scoped hooks install
- **WHEN** init resolves tool adapters for `cline`
- **THEN** Cline detection is based on repository `.clinerules` path presence
- **AND** installation target resolves to the global hooks directory under `~/Documents/Cline/Rules/Hooks/`

### Requirement: Cline Hook-Based Integration
Cline init integration SHALL install a managed `PreToolUse` hook that enforces the `ccp` command-prefix contract before shell execution.

#### Scenario: install managed pre-tool hook
- **WHEN** user runs `ccp init --tools cline`
- **THEN** CCP installs a managed hook in the global Cline hooks directory
- **AND** the hook applies before tool execution
- **AND** the hook blocks unsupported shell execution that does not follow the `ccp` prefix contract

### Requirement: Cline Home Path Discovery
Cline init integration SHALL resolve the global hooks directory from the user's home base path rather than from a hardcoded localized folder string.

#### Scenario: derive hooks path from user home
- **WHEN** user runs `ccp init --tools cline`
- **THEN** CCP derives the global Cline hooks path from the user's home directory or equivalent platform-resolved base path
- **AND** it does not assume a hardcoded folder literal such as `~/Documents` is correct on every system

### Requirement: Cline Idempotent Reapply
Cline adapter SHALL be idempotent on repeated runs.

#### Scenario: re-run cline init
- **WHEN** `ccp init --tools cline` is run twice
- **THEN** the second run does not create duplicate CCP-managed hook artifacts
- **AND** reports no-op or already-configured status when the managed hook content is unchanged.

### Requirement: Cline Hook Uninstall Cleanup
Cline uninstall integration SHALL remove only the CCP-managed Cline hook artifacts.

#### Scenario: uninstall removes managed hook
- **WHEN** uninstall runs after Cline hook integration has been applied
- **THEN** uninstall removes only the CCP-managed Cline hook artifacts
- **AND** preserves unrelated Cline rules and hooks
