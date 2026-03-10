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
- **WHEN** project root contains a known tool directory (for example `.claude`, `.cursor`, `.codex`, `.opencode`, `.github`, `.gemini`, `.amazonq`, `.windsurf`, `.clinerules`, `.continue`, `.trae`, `.kiro`, `.qwen`, `.roo`, `.agent`, `.kilocode`, `.qoder`, `.factory`, `.augment`, `.codebuddy`, `.crush`, `.iflow`, `.pi`)
- **THEN** that tool is included in detected candidates for init selection.

#### Scenario: detect tool by known config file
- **WHEN** project root contains a known tool config file such as `.aider.conf.yml`
- **THEN** that tool is included in detected candidates for init selection.

#### Scenario: ignore non-directory collisions
- **WHEN** a known tool path exists as a file instead of a directory
- **THEN** detection ignores that path and does not treat it as a detected tool.

#### Scenario: no detections without explicit selection
- **WHEN** `--tools` is omitted and no tools are detected
- **THEN** init fails with `no tools detected; specify --tools (...)` guidance.

### Requirement: Canonical Install Scope Resolution
`ccp init` SHALL resolve a canonical managed install target per agent independently from repository-scoped detection.

#### Scenario: repository detection with home-scoped install
- **WHEN** an adapter detects an agent from repository-local markers
- **AND** that agent has a documented home-scoped integration surface
- **THEN** `ccp init` installs CCP-managed integration artifacts at the documented home-scoped target
- **AND** repository-scoped markers remain detection inputs rather than the default install target

#### Scenario: repository scope remains fallback
- **WHEN** an adapter does not have a documented and automatable home-scoped integration surface
- **THEN** `ccp init` keeps the canonical install target at repository scope

### Requirement: Canonical Target May Use Hooks Or Plugins
`ccp init` SHALL prefer the agent-native integration mechanism that provides command interception when that mechanism is documented and automatable.

#### Scenario: hook or plugin surface is preferred over static guidance
- **WHEN** an agent documents a hook or plugin surface that can intercept shell or tool execution
- **THEN** `ccp init` uses that hook or plugin surface as the canonical CCP integration target
- **AND** static rule or instruction files remain fallback or contextual guidance layers only when still needed by that agent

### Requirement: Alias Tools Reuse Canonical Install Targets
Aliases SHALL reuse the canonical install target and behavior of the integration they normalize to.

#### Scenario: alias shares canonical target
- **WHEN** an alias tool is selected for init or uninstall
- **THEN** CCP applies the canonical integration target and lifecycle behavior of the normalized tool
- **AND** it does not create a second competing managed install target for the alias name

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

#### Scenario: continue is a supported adapter-backed tool
- **WHEN** user selects `continue`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Continue adapter.

#### Scenario: trae is a supported adapter-backed tool
- **WHEN** user selects `trae`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Trae adapter.

#### Scenario: kiro is a supported adapter-backed tool
- **WHEN** user selects `kiro`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Kiro adapter.

#### Scenario: roocode is a supported adapter-backed tool
- **WHEN** user selects `roocode`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real RooCode adapter.

#### Scenario: aider is a supported adapter-backed tool
- **WHEN** user selects `aider`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Aider adapter.

#### Scenario: qwen is a supported adapter-backed tool
- **WHEN** user selects `qwen`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Qwen adapter.

#### Scenario: antigravity is a supported adapter-backed tool
- **WHEN** user selects `antigravity`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Antigravity adapter.

#### Scenario: kilocode is a supported adapter-backed tool
- **WHEN** user selects `kilocode`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Kilocode adapter.

#### Scenario: qoder is a supported adapter-backed tool
- **WHEN** user selects `qoder`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Qoder adapter.

#### Scenario: factory is a supported adapter-backed tool
- **WHEN** user selects `factory`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Factory adapter.

#### Scenario: codebuddy is a supported adapter-backed tool
- **WHEN** user selects `codebuddy`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real CodeBuddy adapter.

#### Scenario: auggie is a supported adapter-backed tool
- **WHEN** user selects `auggie`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Auggie adapter.

#### Scenario: costrict is a supported alias tool
- **WHEN** user selects `costrict`
- **THEN** init accepts the tool ID as a valid lifecycle selection
- **AND** routes installation through the existing RooCode integration path rather than a second competing adapter.

#### Scenario: crush is a supported adapter-backed tool
- **WHEN** user selects `crush`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Crush adapter.

#### Scenario: iflow is a supported adapter-backed tool
- **WHEN** user selects `iflow`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real iFlow adapter.

#### Scenario: pi is a supported adapter-backed tool
- **WHEN** user selects `pi`
- **THEN** init accepts the tool ID as a valid registered adapter
- **AND** routes installation through a real Pi adapter.

### Requirement: Adapter Installation Semantics
`ccp init` SHALL invoke adapter planning, installation, and verification for each selected tool, with per-tool states.

#### Scenario: per-tool state reporting
- **WHEN** adapter installation completes for a tool
- **THEN** init records and persists tool state with `status` of `applied`, `noop`, or `failed`
- **AND** includes a reason string.

#### Scenario: adapter failure short-circuits run
- **WHEN** adapter install or verify fails for a selected tool
- **THEN** init returns an error after recording failed state for that tool.

### Requirement: Supported Integrations Install Canonical Managed Guidance

Supported integrations MUST install CCP-managed guidance that preserves the canonical command-prefix instructions and required fallback notes.

#### Scenario: Installed guidance includes raw escape hatch note

- **GIVEN** a user runs `ccp init` for any supported adapter-backed integration
- **WHEN** CCP installs or updates its managed guidance
- **THEN** the managed content tells the agent to prefix shell commands with `ccp`
- **AND** the managed content tells the agent to retry with `ccp --raw` if output seems corrupted, malformed, or unusable for the task

### Requirement: Agent-Specific Artifacts Are Out of Scope
This framework spec SHALL define only generic init orchestration behavior.

#### Scenario: per-agent install details delegated to dedicated specs
- **WHEN** validating concrete artifact paths/content for specific agents
- **THEN** behavior is defined in dedicated specs (for example `init-claude-agent-integration`, `init-codex-agent-integration`, `init-opencode-agent-integration`, `init-github-copilot-agent-integration`, `init-cursor-agent-integration`, `init-gemini-agent-integration`, `init-amazon-q-agent-integration`, `init-windsurf-agent-integration`, `init-cline-agent-integration`, `init-continue-agent-integration`, `init-trae-agent-integration`, `init-kiro-agent-integration`, `init-qwen-agent-integration`, `init-roocode-agent-integration`, `init-aider-agent-integration`, `init-antigravity-agent-integration`, `init-kilocode-agent-integration`, `init-qoder-agent-integration`, `init-factory-agent-integration`, `init-auggie-agent-integration`, `init-codebuddy-agent-integration`, `init-costrict-agent-integration`, `init-crush-agent-integration`, `init-iflow-agent-integration`, `init-pi-agent-integration`)
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
