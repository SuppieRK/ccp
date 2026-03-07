# init-integration-framework Specification

## Purpose
Define `ccp init` behavior for coding-agent detection, installation, and persisted initialization state.

## Requirements
### Requirement: Init Flags
`ccp init` SHALL support explicit tool selection without exposing lifecycle scope selection.

### Requirement: Tool Resolution Order
`ccp init` SHALL resolve tools from explicit selection first, then fall back to adapter-driven detection.

#### Scenario: explicit tools take precedence
- **WHEN** user provides `--tools`
- **THEN** init uses parsed `--tools` values and does not replace them with detected tools.

#### Scenario: parsed tools are normalized
- **WHEN** user provides comma-separated tool IDs in `--tools`
- **THEN** init lowercases, trims whitespace, deduplicates, and sorts selected tool IDs before validation.

#### Scenario: auto-detect when tools flag omitted
- **WHEN** `--tools` is omitted
- **THEN** init detects tools using registered adapters for the selected scope root.

#### Scenario: detect tool by known directory
- **WHEN** project root contains a known tool directory (for example `.claude`, `.cursor`, `.codex`, `.opencode`, `.github`, `.gemini`, `.amazonq`, `.windsurf`, `.clinerules`)
- **THEN** that tool is included in detected candidates for init selection.

#### Scenario: ignore non-directory collisions
- **WHEN** a known tool path exists as a file instead of a directory
- **THEN** detection ignores that path and does not treat it as a detected tool.

#### Scenario: no detections without explicit selection
- **WHEN** `--tools` is omitted and no tools are detected
- **THEN** init fails with `no tools detected; specify --tools (...)` guidance.

### Requirement: Tool Validation
`ccp init` SHALL reject unsupported tool IDs before adapter installation.

#### Scenario: unsupported tool request
- **WHEN** user selects a tool without a registered adapter
- **THEN** init fails with `unsupported tool` diagnostics.

#### Scenario: github-copilot is a supported tool
- **WHEN** user selects `github-copilot`
- **THEN** init accepts the tool ID as a valid registered adapter.

#### Scenario: cursor is a supported adapter-backed tool
- **WHEN** user selects `cursor`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Cursor adapter rather than a no-op placeholder.

#### Scenario: gemini is a supported adapter-backed tool
- **WHEN** user selects `gemini`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Gemini adapter.

#### Scenario: amazon-q is a supported adapter-backed tool
- **WHEN** user selects `amazon-q`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Amazon Q adapter.

#### Scenario: windsurf is a supported adapter-backed tool
- **WHEN** user selects `windsurf`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Windsurf adapter.

#### Scenario: cline is a supported adapter-backed tool
- **WHEN** user selects `cline`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Cline adapter.

### Requirement: Adapter Installation Semantics
`ccp init` SHALL invoke adapter planning, installation, and verification for each selected tool, with per-tool states.

#### Scenario: per-tool state reporting
- **WHEN** adapter installation completes for a tool
- **THEN** init records and persists tool state with `status` of `applied`, `noop`, or `failed`
- **AND** includes a reason string.

#### Scenario: adapter failure short-circuits run
- **WHEN** adapter install or verify fails for a selected tool
- **THEN** init returns an error after recording failed state for that tool.

### Requirement: Agent-Specific Artifacts Are Out of Scope
This framework spec SHALL define only generic init orchestration behavior.

#### Scenario: per-agent install details delegated to dedicated specs
- **WHEN** validating concrete artifact paths/content for specific agents
- **THEN** behavior is defined in dedicated specs (for example `init-claude-agent-integration`, `init-codex-agent-integration`, `init-opencode-agent-integration`, `init-github-copilot-agent-integration`, `init-cursor-agent-integration`, `init-gemini-agent-integration`, `init-amazon-q-agent-integration`, `init-windsurf-agent-integration`, `init-cline-agent-integration`)
- **AND** this framework spec remains agent-agnostic.

### Requirement: Idempotent and Safe Writes
The framework SHALL mutate managed files using idempotent, backup-safe, atomic write semantics.

#### Scenario: unchanged content is no-op
- **WHEN** target file content already matches desired state
- **THEN** framework reports `noop` and does not rewrite the file.

#### Scenario: changed content creates backup then atomically replaces
- **WHEN** target file requires update
- **THEN** framework creates a backup of previous content
- **AND** applies replacement atomically.

### Requirement: Init Configuration Persistence
`ccp init` SHALL persist init configuration at a single managed path.

#### Scenario: persisted init config shape
- **WHEN** init succeeds
- **THEN** it writes config JSON containing `tools` and `state`.

#### Scenario: managed config writes under home config
- **WHEN** init runs
- **THEN** config path is `~/.config/ccp/init.json`.

#### Scenario: detection root remains repository-scoped
- **WHEN** `--tools` is omitted
- **THEN** tool detection still uses the current working directory as the repository root.
