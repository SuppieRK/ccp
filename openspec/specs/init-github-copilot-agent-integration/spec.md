# init-github-copilot-agent-integration Specification

## Purpose
Define GitHub Copilot CLI-specific init integration using a home-scoped managed instructions file and Codex-style CCP-managed block semantics.

## Requirements
### Requirement: GitHub Copilot Detection vs Install Scope
GitHub Copilot init integration SHALL detect from repository scope while installing managed instructions in user home scope.

#### Scenario: repository detection and home-scoped install
- **WHEN** init resolves tool adapters for `github-copilot`
- **THEN** GitHub Copilot detection is based on repository `.github` directory presence
- **AND** installation target remains `~/.copilot/copilot-instructions.md`.

### Requirement: GitHub Copilot User Instructions Target
GitHub Copilot init integration SHALL manage Copilot CLI instructions at `~/.copilot/copilot-instructions.md`.

#### Scenario: deterministic instructions target
- **WHEN** user runs `ccp init --tools github-copilot`
- **THEN** integration resolves `~/.copilot/copilot-instructions.md` as the canonical GitHub Copilot target for installation or update.

### Requirement: GitHub Copilot Managed Instruction Block
GitHub Copilot init integration SHALL upsert a CCP-managed instruction block that routes shell command execution through `ccp`.

#### Scenario: first-run managed block insertion
- **WHEN** user runs `ccp init --tools github-copilot`
- **THEN** `~/.copilot/copilot-instructions.md` contains a managed CCP block with begin and end markers
- **AND** managed content instructs GitHub Copilot to prefer executing shell commands through `ccp`.

#### Scenario: managed block matches Codex contract
- **WHEN** integration writes CCP-managed GitHub Copilot instructions
- **THEN** the managed block content matches the canonical Codex-managed block content exactly
- **AND** it includes the same explicit `ccp` prefix instruction, command examples, and missing-binary fallback note.

#### Scenario: preserve non-managed content
- **WHEN** `~/.copilot/copilot-instructions.md` already contains user-authored content outside managed markers
- **THEN** GitHub Copilot integration preserves all non-managed content unchanged.

### Requirement: GitHub Copilot Managed Block Markers
GitHub Copilot integration SHALL use CCP-specific stable markers for deterministic managed-block upsert behavior.

#### Scenario: marker stability
- **WHEN** managed block is written for GitHub Copilot
- **THEN** it uses canonical markers:
  - `<!-- BEGIN: CCP MANAGED BLOCK -->`
  - `<!-- END: CCP MANAGED BLOCK -->`
- **AND** reruns replace only content inside markers.

### Requirement: GitHub Copilot Idempotent Reapply
GitHub Copilot adapter SHALL be idempotent on repeated runs.

#### Scenario: re-run GitHub Copilot init
- **WHEN** `ccp init --tools github-copilot` is run twice
- **THEN** the second run does not duplicate managed block content
- **AND** reports no-op or already-configured status.

### Requirement: GitHub Copilot Uninstall Cleanup
GitHub Copilot uninstall integration SHALL remove only the CCP-managed instruction block while preserving unrelated user content.

#### Scenario: uninstall removes managed block only
- **WHEN** uninstall runs and `~/.copilot/copilot-instructions.md` contains unrelated content outside the CCP-managed block
- **THEN** uninstall removes only the CCP-managed block
- **AND** preserves the unrelated content unchanged.

#### Scenario: uninstall removes file when block is only content
- **WHEN** uninstall runs and the CCP-managed block is the only effective content in `~/.copilot/copilot-instructions.md`
- **THEN** uninstall removes `~/.copilot/copilot-instructions.md`.

### Requirement: Shared Instruction-File Integration Contract
GitHub Copilot instruction-file integration SHALL use the same reusable managed-block lifecycle contract as Codex while allowing tool-specific detection and target paths.

#### Scenario: shared instruction-file lifecycle behavior
- **WHEN** GitHub Copilot and Codex integrations are implemented
- **THEN** both use the same managed-block upsert, verify, and uninstall semantics
- **AND** only detection roots, target paths, and adapter IDs vary by tool.
