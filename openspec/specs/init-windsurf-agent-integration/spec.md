# init-windsurf-agent-integration Specification

## Purpose
Define the managed Windsurf CLI integration installed and removed by `ccp init` and `ccp uninstall`.

## Requirements
### Requirement: Windsurf Detection vs Install Scope
Windsurf init integration SHALL detect from repository scope and install to a user-scoped hooks target.

#### Scenario: repository detection with user-scoped hooks install
- **WHEN** init resolves tool adapters for `windsurf`
- **THEN** Windsurf detection is based on repository `.windsurf` directory presence
- **AND** installation target resolves to `~/.codeium/windsurf/hooks.json`

### Requirement: Windsurf Hook-Based Integration
Windsurf init integration SHALL install a managed user-scoped `pre_run_command` hook that enforces the `ccp` prefix contract before shell execution.

#### Scenario: install managed windsurf pre-run hook
- **WHEN** user runs `ccp init --tools windsurf`
- **THEN** CCP writes a managed `pre_run_command` hook contribution into `~/.codeium/windsurf/hooks.json`
- **AND** that hook blocks unsupported shell execution that does not follow the `ccp` prefix contract

#### Scenario: hook runtime uses only bash builtins and ccp
- **WHEN** the managed Windsurf hook executes
- **THEN** its runtime behavior does not depend on helper commands other than bash builtins and `ccp`

#### Scenario: exit paths leave troubleshooting markers
- **WHEN** the managed Windsurf hook exits with status `0`
- **THEN** it appends a deterministic reason marker to a log file under the system tmp directory
- **AND** each early-return branch uses a distinct marker

### Requirement: Windsurf Idempotent Reapply
Windsurf adapter SHALL be idempotent on repeated runs.

#### Scenario: re-run windsurf init
- **WHEN** `ccp init --tools windsurf` is run twice
- **THEN** the second run does not create duplicate CCP-managed hook contributions
- **AND** reports no-op or already-configured status when the managed hook content is unchanged.

### Requirement: Windsurf Hook Uninstall Preserves Unrelated Hooks
Windsurf uninstall integration SHALL remove only the CCP-managed hook contribution from the Windsurf hooks file.

#### Scenario: uninstall preserves unrelated windsurf hooks
- **WHEN** uninstall runs after Windsurf integration has been applied
- **THEN** uninstall removes only the CCP-managed hook contribution from `~/.codeium/windsurf/hooks.json`
- **AND** preserves unrelated Windsurf hooks
