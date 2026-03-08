# init-kilocode-agent-integration Specification

## Purpose
Define the managed Kilocode integration installed and removed by `ccp init` and `ccp uninstall`.

## Requirements
### Requirement: Kilocode Detection vs Install Scope
Kilocode init integration SHALL detect from repository scope and install to a repository-scoped Kilocode rules target.

#### Scenario: repository detection with repository-scoped install
- **WHEN** init resolves tool adapters for `kilocode`
- **THEN** Kilocode detection is based on repository `.kilocode` directory presence
- **AND** installation target remains under the repository at `.kilocode/rules/ccp.md`.

### Requirement: Kilocode Managed Rule Target
Kilocode init integration SHALL manage a deterministic workspace rule file at `.kilocode/rules/ccp.md`.

#### Scenario: deterministic managed target
- **WHEN** user runs `ccp init --tools kilocode`
- **THEN** integration resolves `.kilocode/rules/ccp.md` as the canonical Kilocode target for installation or update.

### Requirement: Kilocode Managed Rule Content
Kilocode init integration SHALL install Kilocode guidance that routes shell command execution through `ccp`.

#### Scenario: first-run managed rule creation
- **WHEN** user runs `ccp init --tools kilocode`
- **THEN** `.kilocode/rules/ccp.md` is created or updated
- **AND** the managed rule instructs Kilocode to prefer executing shell commands through `ccp`.

#### Scenario: canonical ccp guidance is preserved
- **WHEN** integration writes the managed Kilocode rule
- **THEN** the rule body preserves canonical CCP shell guidance where possible
- **AND** includes the `ccp --raw` escape-hatch note.

### Requirement: Kilocode Dedicated Managed Rule File
Kilocode init integration SHALL use a dedicated managed rule file rather than a managed block inside a shared instruction document.

#### Scenario: managed file is wholly owned by CCP
- **WHEN** Kilocode integration is installed
- **THEN** CCP manages `.kilocode/rules/ccp.md` as a dedicated rule file
- **AND** it does not upsert managed block markers into unrelated Kilocode files.

### Requirement: Kilocode Idempotent Reapply
Kilocode adapter SHALL be idempotent on repeated runs.

#### Scenario: re-run kilocode init
- **WHEN** `ccp init --tools kilocode` is run twice
- **THEN** the second run does not create duplicate CCP rule files
- **AND** reports no-op or already-configured status when the managed rule content is unchanged.

### Requirement: Kilocode Uninstall Cleanup
Kilocode uninstall integration SHALL remove only the CCP-managed Kilocode rule file.

#### Scenario: uninstall removes managed rule file
- **WHEN** uninstall runs after Kilocode integration has been applied
- **THEN** uninstall removes `.kilocode/rules/ccp.md`.

#### Scenario: uninstall preserves other Kilocode project files
- **WHEN** uninstall removes the managed Kilocode rule file
- **THEN** it does not remove other files under `.kilocode/`
- **AND** it does not prune `.kilocode/rules` directories solely because they become empty.
