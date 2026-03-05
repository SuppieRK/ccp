# init-codex-agent-integration Specification

## Purpose
Define Codex-specific init integration using global AGENTS managed instructions and shared init-router agent installer architecture.

## Requirements
### Requirement: Codex Detection vs Install Scope
Codex adapter SHALL detect from repository scope while installing managed instructions in user home scope.

#### Scenario: repository detection and home-scoped install
- **WHEN** init resolves tool adapters for codex
- **THEN** codex detection is based on repository `.codex` presence
- **AND** codex installation target remains `~/.codex/AGENTS.md`.

### Requirement: Codex Global AGENTS Target
Codex init integration SHALL manage Codex global instructions at `~/.codex/AGENTS.md`.

#### Scenario: deterministic global target
- **WHEN** user runs `ccp init --tools codex`
- **THEN** integration resolves `~/.codex/AGENTS.md` as the canonical codex target for installation/update.

### Requirement: Codex Managed Instruction Block
Codex init integration SHALL upsert a CCP-managed instruction block that routes shell command execution through `ccp`.

#### Scenario: first-run managed block insertion
- **WHEN** user runs `ccp init --tools codex`
- **THEN** `~/.codex/AGENTS.md` contains a managed CCP block with begin/end markers
- **AND** managed content instructs Codex to prefer executing shell commands through `ccp`.

#### Scenario: managed block wording contract
- **WHEN** integration writes CCP-managed Codex instructions
- **THEN** the managed block remains concise and includes:
  - explicit instruction to use `ccp` as the command prefix for every executable in shell commands, including chained (`&&`, `||`) and piped (`|`) expressions
  - at least six command examples, including chaining examples with `&&` and `||`
  - fallback note only for missing `ccp` binary.

#### Scenario: canonical managed block template
- **WHEN** integration renders the managed block
- **THEN** it uses this canonical content:

```md
<!-- BEGIN: CCP MANAGED BLOCK -->
## CCP Integration (Managed)

Use `ccp` as the command prefix for every executable in shell commands, including chained (`&&`, `||`) and piped (`|`) expressions.

Examples:
- `ccp ls -la`
- `ccp git status --short`
- `ccp go test -count=1 ./...`
- `ccp echo chain-ok && ccp echo chain-done`
- `ccp false || ccp echo chain-recovered`
- `ccp nl -ba spec.md | ccp sed -n '1,260p'`

If `ccp` is unavailable, run the original command and note that CCP is not installed.
<!-- END: CCP MANAGED BLOCK -->
```

#### Scenario: preserve non-managed content
- **WHEN** `~/.codex/AGENTS.md` already contains user-authored content outside managed markers
- **THEN** codex integration preserves all non-managed content unchanged.

### Requirement: Codex Idempotent Reapply
Codex adapter SHALL be idempotent on repeated runs.

#### Scenario: Re-run codex init
- **WHEN** `ccp init --tools codex` is run twice
- **THEN** second run does not duplicate managed block content
- **AND** reports no-op or already-configured status.

### Requirement: Codex Managed Block Markers
Codex integration SHALL use stable markers for deterministic managed-block upsert behavior.

#### Scenario: marker stability
- **WHEN** managed block is written
- **THEN** it uses canonical markers:
  - `<!-- BEGIN: CCP MANAGED BLOCK -->`
  - `<!-- END: CCP MANAGED BLOCK -->`
- **AND** reruns replace only content inside markers.

### Requirement: Generic Init Router and Agent Installer Contract
Init lifecycle SHALL support agent-specific installation through a shared installer contract with router-only orchestration in `init.go`.

#### Scenario: init.go delegates to installer
- **WHEN** `ccp init` runs for selected tools
- **THEN** `internal/lifecycle/init.go` dispatches selected agent IDs to per-agent install methods
- **AND** does not embed agent-specific file wiring rules inline.

#### Scenario: shared agent enum governs routing
- **WHEN** tools are parsed and validated
- **THEN** selection and dispatch use a shared supported-agent enum/registry.
