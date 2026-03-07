# init-amazon-q-agent-integration Specification

## Purpose
Define the managed Amazon Q CLI integration installed and removed by `ccp init` and `ccp uninstall`.

## Requirements
### Requirement: Amazon Q Detection vs Install Scope
Amazon Q init integration SHALL detect from repository scope and install to a repository-scoped Amazon Q rules target.

#### Scenario: repository detection with repository-scoped install
- **WHEN** init resolves tool adapters for `amazon-q`
- **THEN** Amazon Q detection is based on repository `.amazonq` directory presence
- **AND** installation target remains under the repository at `.amazonq/rules/ccp.md`.

### Requirement: Amazon Q Managed Rule Target
Amazon Q init integration SHALL manage a deterministic project rule file at `.amazonq/rules/ccp.md`.

#### Scenario: deterministic managed target
- **WHEN** user runs `ccp init --tools amazon-q`
- **THEN** integration resolves `.amazonq/rules/ccp.md` as the canonical Amazon Q target for installation or update.

### Requirement: Amazon Q Managed Rule Content
Amazon Q init integration SHALL install Amazon Q CLI guidance that routes shell command execution through `ccp`.

#### Scenario: first-run managed rule creation
- **WHEN** user runs `ccp init --tools amazon-q`
- **THEN** `.amazonq/rules/ccp.md` is created or updated
- **AND** the managed rule instructs Amazon Q CLI to prefer executing shell commands through `ccp`.

#### Scenario: canonical CCP guidance is preserved
- **WHEN** integration writes the managed Amazon Q rule
- **THEN** it preserves the same behavioral guidance as the other managed agent integrations where possible
- **AND** it includes explicit `ccp` prefix instruction, command-shape examples, and missing-binary fallback guidance.

### Requirement: Amazon Q Dedicated Managed Rule File
Amazon Q init integration SHALL use a dedicated managed rule file rather than a managed block inside a shared instruction document.

#### Scenario: managed file is wholly owned by CCP
- **WHEN** Amazon Q integration is installed
- **THEN** CCP manages `.amazonq/rules/ccp.md` as a dedicated rule file
- **AND** it does not upsert managed block markers into unrelated Amazon Q rule files.

### Requirement: Amazon Q Idempotent Reapply
Amazon Q adapter SHALL be idempotent on repeated runs.

#### Scenario: re-run amazon-q init
- **WHEN** `ccp init --tools amazon-q` is run twice
- **THEN** the second run does not create duplicate CCP rule files
- **AND** reports no-op or already-configured status when the managed rule content is unchanged.

### Requirement: Amazon Q Uninstall Cleanup
Amazon Q uninstall integration SHALL remove only the CCP-managed Amazon Q rule file.

#### Scenario: uninstall removes managed rule file
- **WHEN** uninstall runs after Amazon Q integration has been applied
- **THEN** uninstall removes `.amazonq/rules/ccp.md`.

#### Scenario: uninstall preserves other Amazon Q project files
- **WHEN** uninstall removes the managed Amazon Q rule file
- **THEN** it does not remove other files under `.amazonq/`
- **AND** it does not prune `.amazonq/rules` directories solely because they become empty.
