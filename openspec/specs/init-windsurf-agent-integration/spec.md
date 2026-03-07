# init-windsurf-agent-integration Specification

## Purpose
Define the managed Windsurf CLI integration installed and removed by `ccp init` and `ccp uninstall`.

## Requirements
### Requirement: Windsurf Detection vs Install Scope
Windsurf init integration SHALL detect from repository scope and install to a repository-scoped Windsurf rules target.

#### Scenario: repository detection with repository-scoped install
- **WHEN** init resolves tool adapters for `windsurf`
- **THEN** Windsurf detection is based on repository `.windsurf` directory presence
- **AND** installation target remains under the repository at `.windsurf/rules/ccp.md`.

### Requirement: Windsurf Managed Rule Target
Windsurf init integration SHALL manage a deterministic workspace rule file at `.windsurf/rules/ccp.md`.

#### Scenario: deterministic managed target
- **WHEN** user runs `ccp init --tools windsurf`
- **THEN** integration resolves `.windsurf/rules/ccp.md` as the canonical Windsurf target for installation or update.

### Requirement: Windsurf Managed Rule Content
Windsurf init integration SHALL install Windsurf CLI guidance that routes shell command execution through `ccp`.

#### Scenario: first-run managed rule creation
- **WHEN** user runs `ccp init --tools windsurf`
- **THEN** `.windsurf/rules/ccp.md` is created or updated
- **AND** the managed rule instructs Windsurf CLI to prefer executing shell commands through `ccp`.

#### Scenario: Windsurf-native metadata is present
- **WHEN** integration writes the managed Windsurf rule
- **THEN** it uses Windsurf-native rule metadata with `trigger: always_on`
- **AND** the rule body preserves canonical CCP shell guidance where possible.

### Requirement: Windsurf Dedicated Managed Rule File
Windsurf init integration SHALL use a dedicated managed rule file rather than a managed block inside a shared instruction document.

#### Scenario: managed file is wholly owned by CCP
- **WHEN** Windsurf integration is installed
- **THEN** CCP manages `.windsurf/rules/ccp.md` as a dedicated rule file
- **AND** it does not upsert managed block markers into unrelated Windsurf files.

### Requirement: Windsurf Idempotent Reapply
Windsurf adapter SHALL be idempotent on repeated runs.

#### Scenario: re-run windsurf init
- **WHEN** `ccp init --tools windsurf` is run twice
- **THEN** the second run does not create duplicate CCP rule files
- **AND** reports no-op or already-configured status when the managed rule content is unchanged.

### Requirement: Windsurf Uninstall Cleanup
Windsurf uninstall integration SHALL remove only the CCP-managed Windsurf rule file.

#### Scenario: uninstall removes managed rule file
- **WHEN** uninstall runs after Windsurf integration has been applied
- **THEN** uninstall removes `.windsurf/rules/ccp.md`.

#### Scenario: uninstall preserves other Windsurf project files
- **WHEN** uninstall removes the managed Windsurf rule file
- **THEN** it does not remove other files under `.windsurf/`
- **AND** it does not prune `.windsurf/rules` directories solely because they become empty.
