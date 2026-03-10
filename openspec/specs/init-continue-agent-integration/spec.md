# init-continue-agent-integration Specification

## Purpose
Define the managed Continue integration installed and removed by `ccp init` and `ccp uninstall`.

## Requirements
### Requirement: Continue Detection vs Install Scope
Continue init integration SHALL detect from repository scope and install to a home-scoped Continue settings target.

#### Scenario: repository detection with home-scoped settings install
- **WHEN** init resolves tool adapters for `continue`
- **THEN** Continue detection is based on repository `.continue` directory presence
- **AND** installation target resolves to `~/.continue/settings.json`

### Requirement: Continue Hook-Based Integration
Continue init integration SHALL install CCP through Continue CLI's Claude-compatible hook system.

#### Scenario: install pre-tool-use hook in global settings
- **WHEN** the user runs `ccp init --tools continue`
- **THEN** CCP writes a managed `PreToolUse` hook contribution into `~/.continue/settings.json`
- **AND** that hook routes shell command execution through `ccp`

#### Scenario: hook runtime uses only bash builtins and ccp
- **WHEN** the managed Continue hook executes
- **THEN** its runtime behavior does not depend on helper commands other than bash builtins and `ccp`

#### Scenario: exit paths leave troubleshooting markers
- **WHEN** the managed Continue hook exits with status `0`
- **THEN** it appends a deterministic reason marker to a log file under the system tmp directory
- **AND** each early-return branch uses a distinct marker

### Requirement: Continue Idempotent Reapply
Continue adapter SHALL be idempotent on repeated runs.

#### Scenario: re-run continue init
- **WHEN** `ccp init --tools continue` is run twice
- **THEN** the second run does not create duplicate CCP-managed hook settings or scripts
- **AND** reports no-op or already-configured status when the managed content is unchanged.

### Requirement: Continue Hook Uninstall Preserves Unrelated Settings
Continue uninstall integration SHALL remove only the managed hook contribution from Continue settings.

#### Scenario: uninstall preserves unrelated settings
- **WHEN** the user uninstalls Continue integration
- **THEN** CCP removes only the CCP-managed hook contribution from `~/.continue/settings.json`
- **AND** preserves unrelated Continue settings
