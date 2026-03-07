# init-gemini-agent-integration Specification

## Purpose
Define Gemini CLI-specific managed installation, verification, and uninstall behavior for `ccp init`.

## Requirements
### Requirement: Gemini Detection vs Install Scope
Gemini init integration SHALL detect from repository scope and install to a user-scoped Gemini context target.

#### Scenario: repository detection with home-scoped install
- **WHEN** init resolves tool adapters for `gemini`
- **THEN** Gemini detection is based on repository `.gemini` directory presence
- **AND** installation target remains under the user home directory at `~/.gemini/GEMINI.md`.

### Requirement: Gemini Managed Context Target
Gemini init integration SHALL manage CCP-owned content inside `~/.gemini/GEMINI.md`.

#### Scenario: deterministic managed target
- **WHEN** user runs `ccp init --tools gemini`
- **THEN** integration resolves `~/.gemini/GEMINI.md` as the canonical Gemini target for installation or update.

### Requirement: Gemini Managed Context Content
Gemini init integration SHALL install Gemini CLI guidance that routes shell command execution through `ccp`.

#### Scenario: first-run managed block creation
- **WHEN** user runs `ccp init --tools gemini`
- **THEN** `~/.gemini/GEMINI.md` is created or updated
- **AND** the managed content instructs Gemini CLI to prefer executing shell commands through `ccp`.

#### Scenario: canonical CCP guidance is preserved
- **WHEN** integration writes the managed Gemini content
- **THEN** it preserves the same behavioral guidance as Codex and GitHub Copilot integrations
- **AND** it includes explicit `ccp` prefix instruction, command-shape examples, and missing-binary fallback guidance.

### Requirement: Gemini Managed Block Semantics
Gemini init integration SHALL use the canonical CCP-managed block markers and upsert semantics.

#### Scenario: managed block markers are CCP-specific
- **WHEN** Gemini managed content is installed
- **THEN** the inserted block uses the same CCP-managed markers as other managed instruction-file integrations
- **AND** marker ownership is scoped to CCP rather than Gemini-specific wording.

#### Scenario: reapply replaces only the managed block
- **WHEN** `~/.gemini/GEMINI.md` already contains a CCP-managed block
- **THEN** re-running init replaces only the managed block content
- **AND** preserves surrounding non-CCP content.

### Requirement: Gemini Idempotent Reapply
Gemini adapter SHALL be idempotent on repeated runs.

#### Scenario: re-run gemini init
- **WHEN** `ccp init --tools gemini` is run twice
- **THEN** the second run does not duplicate the CCP-managed block
- **AND** reports no-op or already-configured status when the managed content is unchanged.

### Requirement: Gemini Uninstall Cleanup
Gemini uninstall integration SHALL remove only the CCP-managed block from `~/.gemini/GEMINI.md`.

#### Scenario: uninstall removes only managed block
- **WHEN** uninstall runs after Gemini integration has been applied
- **THEN** uninstall removes only the CCP-managed block from `~/.gemini/GEMINI.md`.

#### Scenario: uninstall removes file only when managed block was sole content
- **WHEN** the managed block is the only content in `~/.gemini/GEMINI.md`
- **THEN** uninstall removes the file entirely.

#### Scenario: uninstall preserves user-authored Gemini context
- **WHEN** `~/.gemini/GEMINI.md` contains non-CCP content before or after the managed block
- **THEN** uninstall preserves the non-CCP content.
