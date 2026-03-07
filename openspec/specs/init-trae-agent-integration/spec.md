# init-trae-agent-integration Specification

## Purpose
Define the managed Trae integration installed and removed by `ccp init` and `ccp uninstall`.

## Requirements
### Requirement: Trae Detection vs Install Scope
Trae init integration SHALL detect from repository scope and install to a repository-scoped Trae rules target.

#### Scenario: repository detection with repository-scoped install
- **WHEN** init resolves tool adapters for `trae`
- **THEN** Trae detection is based on repository `.trae` directory presence
- **AND** installation target remains under the repository at `.trae/rules/ccp.md`.

### Requirement: Trae Managed Rule Target
Trae init integration SHALL manage a deterministic project rule file at `.trae/rules/ccp.md`.

#### Scenario: deterministic managed target
- **WHEN** user runs `ccp init --tools trae`
- **THEN** integration resolves `.trae/rules/ccp.md` as the canonical Trae target for installation or update.

### Requirement: Trae Managed Rule Content
Trae init integration SHALL install Trae guidance that routes shell command execution through `ccp`.

#### Scenario: first-run managed rule creation
- **WHEN** user runs `ccp init --tools trae`
- **THEN** `.trae/rules/ccp.md` is created or updated
- **AND** the managed rule instructs Trae to prefer executing shell commands through `ccp`.

#### Scenario: canonical CCP guidance is preserved
- **WHEN** integration writes the managed Trae rule
- **THEN** it preserves the same behavioral guidance as the other managed agent integrations where possible
- **AND** it includes explicit `ccp` prefix instruction, command-shape examples, and missing-binary fallback guidance.

### Requirement: Trae Dedicated Managed Rule File
Trae init integration SHALL use a dedicated managed rule file rather than a managed block inside a shared instruction document.

#### Scenario: managed file is wholly owned by CCP
- **WHEN** Trae integration is installed
- **THEN** CCP manages `.trae/rules/ccp.md` as a dedicated rule file
- **AND** it does not upsert managed block markers into unrelated Trae files.

### Requirement: Trae Idempotent Reapply
Trae adapter SHALL be idempotent on repeated runs.

#### Scenario: re-run trae init
- **WHEN** `ccp init --tools trae` is run twice
- **THEN** the second run does not create duplicate CCP rule files
- **AND** reports no-op or already-configured status when the managed rule content is unchanged.

### Requirement: Trae Uninstall Cleanup
Trae uninstall integration SHALL remove only the CCP-managed Trae rule file.

#### Scenario: uninstall removes managed rule file
- **WHEN** uninstall runs after Trae integration has been applied
- **THEN** uninstall removes `.trae/rules/ccp.md`.

#### Scenario: uninstall preserves other Trae project files
- **WHEN** uninstall removes the managed Trae rule file
- **THEN** it does not remove other files under `.trae/`
- **AND** it does not prune `.trae/rules` directories solely because they become empty.
