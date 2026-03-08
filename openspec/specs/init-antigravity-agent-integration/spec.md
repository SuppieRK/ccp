# init-antigravity-agent-integration Specification

## Purpose
Define the managed Antigravity integration installed and removed by `ccp init` and `ccp uninstall`.

## Requirements
### Requirement: Antigravity Detection vs Install Scope
Antigravity init integration SHALL detect from repository scope and install to a repository-scoped Antigravity rules target.

#### Scenario: repository detection with repository-scoped install
- **WHEN** init resolves tool adapters for `antigravity`
- **THEN** Antigravity detection is based on repository `.agent` directory presence
- **AND** installation target remains under the repository at `.agent/rules/ccp.md`.

### Requirement: Antigravity Managed Rule Target
Antigravity init integration SHALL manage a deterministic workspace rule file at `.agent/rules/ccp.md`.

#### Scenario: deterministic managed target
- **WHEN** user runs `ccp init --tools antigravity`
- **THEN** integration resolves `.agent/rules/ccp.md` as the canonical Antigravity target for installation or update.

### Requirement: Antigravity Managed Rule Content
Antigravity init integration SHALL install Antigravity guidance that routes shell command execution through `ccp`.

#### Scenario: first-run managed rule creation
- **WHEN** user runs `ccp init --tools antigravity`
- **THEN** `.agent/rules/ccp.md` is created or updated
- **AND** the managed rule instructs Antigravity to prefer executing shell commands through `ccp`.

#### Scenario: canonical ccp guidance is preserved
- **WHEN** integration writes the managed Antigravity rule
- **THEN** the rule body preserves canonical CCP shell guidance where possible
- **AND** includes the `ccp --raw` escape-hatch note.

### Requirement: Antigravity Dedicated Managed Rule File
Antigravity init integration SHALL use a dedicated managed rule file rather than a managed block inside a shared instruction document.

#### Scenario: managed file is wholly owned by CCP
- **WHEN** Antigravity integration is installed
- **THEN** CCP manages `.agent/rules/ccp.md` as a dedicated rule file
- **AND** it does not upsert managed block markers into unrelated Antigravity files.

### Requirement: Antigravity Idempotent Reapply
Antigravity adapter SHALL be idempotent on repeated runs.

#### Scenario: re-run antigravity init
- **WHEN** `ccp init --tools antigravity` is run twice
- **THEN** the second run does not create duplicate CCP rule files
- **AND** reports no-op or already-configured status when the managed rule content is unchanged.

### Requirement: Antigravity Uninstall Cleanup
Antigravity uninstall integration SHALL remove only the CCP-managed Antigravity rule file.

#### Scenario: uninstall removes managed rule file
- **WHEN** uninstall runs after Antigravity integration has been applied
- **THEN** uninstall removes `.agent/rules/ccp.md`.

#### Scenario: uninstall preserves other Antigravity project files
- **WHEN** uninstall removes the managed Antigravity rule file
- **THEN** it does not remove other files under `.agent/`
- **AND** it does not prune `.agent/rules` directories solely because they become empty.
