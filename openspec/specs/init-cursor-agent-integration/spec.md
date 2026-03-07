# init-cursor-agent-integration Specification

## Purpose
Define Cursor CLI-specific managed installation, verification, and uninstall behavior for `ccp init`.

## Requirements
### Requirement: Cursor Detection vs Install Scope
Cursor init integration SHALL detect and install from repository scope.

#### Scenario: repository detection and repository-scoped install
- **WHEN** init resolves tool adapters for `cursor`
- **THEN** Cursor detection is based on repository `.cursor` directory presence
- **AND** installation target remains under the repository at `.cursor/rules/ccp.mdc`.

### Requirement: Cursor Managed Rule Target
Cursor init integration SHALL manage a deterministic Cursor rule file at `.cursor/rules/ccp.mdc`.

#### Scenario: deterministic rule target
- **WHEN** user runs `ccp init --tools cursor`
- **THEN** integration resolves `.cursor/rules/ccp.mdc` as the canonical Cursor target for installation or update.

### Requirement: Cursor Managed Rule Content
Cursor init integration SHALL install Cursor-native rule content that routes shell command execution through `ccp`.

#### Scenario: first-run managed rule creation
- **WHEN** user runs `ccp init --tools cursor`
- **THEN** `.cursor/rules/ccp.mdc` is created or updated
- **AND** the managed rule instructs Cursor CLI to prefer executing shell commands through `ccp`.

#### Scenario: canonical CCP guidance is preserved
- **WHEN** integration writes the managed Cursor rule
- **THEN** the rule preserves the same behavioral guidance as Codex and GitHub Copilot integrations
- **AND** it includes explicit `ccp` prefix instruction, command-shape examples, and missing-binary fallback guidance adapted to Cursor rule format.

### Requirement: Cursor Rule Metadata Is Minimal
Cursor init integration SHALL use the minimum Cursor rule metadata required for the managed rule to apply as a project rule.

#### Scenario: minimal always-on metadata
- **WHEN** `.cursor/rules/ccp.mdc` is rendered
- **THEN** it includes only the minimal frontmatter or metadata needed for Cursor to load it as an always-applied project rule
- **AND** it avoids extra categorization or scope controls not required for CCP integration.

### Requirement: Cursor Idempotent Reapply
Cursor adapter SHALL be idempotent on repeated runs.

#### Scenario: re-run cursor init
- **WHEN** `ccp init --tools cursor` is run twice
- **THEN** the second run does not create duplicate CCP rule files
- **AND** reports no-op or already-configured status when the managed rule content is unchanged.

### Requirement: Cursor Uninstall Cleanup
Cursor uninstall integration SHALL remove only the CCP-managed Cursor rule file.

#### Scenario: uninstall removes managed rule file
- **WHEN** uninstall runs after Cursor integration has been applied
- **THEN** uninstall removes `.cursor/rules/ccp.mdc`.

#### Scenario: uninstall preserves other Cursor project files
- **WHEN** uninstall removes the managed Cursor rule file
- **THEN** it does not remove other files under `.cursor/`
- **AND** it does not prune `.cursor/rules` directories solely because they become empty.
