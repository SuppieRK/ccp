# init-cline-agent-integration Specification

## Purpose
Define the managed Cline integration installed and removed by `ccp init` and `ccp uninstall`.

## Requirements
### Requirement: Cline Detection vs Install Scope
Cline init integration SHALL detect from repository scope and install to a repository-scoped Cline rules target.

#### Scenario: repository detection with repository-scoped install
- **WHEN** init resolves tool adapters for `cline`
- **THEN** Cline detection is based on repository `.clinerules` path presence
- **AND** installation target remains under the repository at `.clinerules/ccp.md`.

### Requirement: Cline Managed Rule Target
Cline init integration SHALL manage a deterministic rule file at `.clinerules/ccp.md`.

#### Scenario: deterministic managed target
- **WHEN** user runs `ccp init --tools cline`
- **THEN** integration resolves `.clinerules/ccp.md` as the canonical Cline target for installation or update.

### Requirement: Cline Managed Rule Content
Cline init integration SHALL install Cline guidance that routes shell command execution through `ccp`.

#### Scenario: first-run managed rule creation
- **WHEN** user runs `ccp init --tools cline`
- **THEN** `.clinerules/ccp.md` is created or updated
- **AND** the managed rule instructs Cline to prefer executing shell commands through `ccp`.

#### Scenario: canonical CCP guidance is preserved
- **WHEN** integration writes the managed Cline rule
- **THEN** it preserves the same behavioral guidance as the other managed agent integrations where possible
- **AND** it includes explicit `ccp` prefix instruction, command-shape examples, and missing-binary fallback guidance.

### Requirement: Cline Dedicated Managed Rule File
Cline init integration SHALL use a dedicated managed rule file rather than a managed block inside a shared instruction document.

#### Scenario: managed file is wholly owned by CCP
- **WHEN** Cline integration is installed
- **THEN** CCP manages `.clinerules/ccp.md` as a dedicated rule file
- **AND** it does not upsert managed block markers into unrelated Cline files.

### Requirement: Cline Idempotent Reapply
Cline adapter SHALL be idempotent on repeated runs.

#### Scenario: re-run cline init
- **WHEN** `ccp init --tools cline` is run twice
- **THEN** the second run does not create duplicate CCP rule files
- **AND** reports no-op or already-configured status when the managed rule content is unchanged.

### Requirement: Cline Uninstall Cleanup
Cline uninstall integration SHALL remove only the CCP-managed Cline rule file.

#### Scenario: uninstall removes managed rule file
- **WHEN** uninstall runs after Cline integration has been applied
- **THEN** uninstall removes `.clinerules/ccp.md`.

#### Scenario: uninstall preserves other Cline project files
- **WHEN** uninstall removes the managed Cline rule file
- **THEN** it does not remove other files under `.clinerules`
- **AND** it does not prune parent directories solely because they become empty.
