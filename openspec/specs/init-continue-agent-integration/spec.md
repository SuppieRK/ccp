# init-continue-agent-integration Specification

## Purpose
Define the managed Continue integration installed and removed by `ccp init` and `ccp uninstall`.

## Requirements
### Requirement: Continue Detection vs Install Scope
Continue init integration SHALL detect from repository scope and install to a repository-scoped Continue rules target.

#### Scenario: repository detection with repository-scoped install
- **WHEN** init resolves tool adapters for `continue`
- **THEN** Continue detection is based on repository `.continue` directory presence
- **AND** installation target remains under the repository at `.continue/rules/ccp.md`.

### Requirement: Continue Managed Rule Target
Continue init integration SHALL manage a deterministic project rule file at `.continue/rules/ccp.md`.

#### Scenario: deterministic managed target
- **WHEN** user runs `ccp init --tools continue`
- **THEN** integration resolves `.continue/rules/ccp.md` as the canonical Continue target for installation or update.

### Requirement: Continue Managed Rule Content
Continue init integration SHALL install Continue guidance that routes shell command execution through `ccp`.

#### Scenario: first-run managed rule creation
- **WHEN** user runs `ccp init --tools continue`
- **THEN** `.continue/rules/ccp.md` is created or updated
- **AND** the managed rule instructs Continue to prefer executing shell commands through `ccp`.

#### Scenario: canonical CCP guidance is preserved
- **WHEN** integration writes the managed Continue rule
- **THEN** it preserves the same behavioral guidance as the other managed agent integrations where possible
- **AND** it includes explicit `ccp` prefix instruction, command-shape examples, and missing-binary fallback guidance.

### Requirement: Continue Dedicated Managed Rule File
Continue init integration SHALL use a dedicated managed rule file rather than a managed block inside a shared instruction document.

#### Scenario: managed file is wholly owned by CCP
- **WHEN** Continue integration is installed
- **THEN** CCP manages `.continue/rules/ccp.md` as a dedicated rule file
- **AND** it does not upsert managed block markers into unrelated Continue files.

### Requirement: Continue Idempotent Reapply
Continue adapter SHALL be idempotent on repeated runs.

#### Scenario: re-run continue init
- **WHEN** `ccp init --tools continue` is run twice
- **THEN** the second run does not create duplicate CCP rule files
- **AND** reports no-op or already-configured status when the managed rule content is unchanged.

### Requirement: Continue Uninstall Cleanup
Continue uninstall integration SHALL remove only the CCP-managed Continue rule file.

#### Scenario: uninstall removes managed rule file
- **WHEN** uninstall runs after Continue integration has been applied
- **THEN** uninstall removes `.continue/rules/ccp.md`.

#### Scenario: uninstall preserves other Continue project files
- **WHEN** uninstall removes the managed Continue rule file
- **THEN** it does not remove other files under `.continue/`
- **AND** it does not prune `.continue/rules` directories solely because they become empty.
