## Purpose
Define `pnpm` filter prepare routing and runtime compaction behavior.

## Requirements

### Requirement: pnpm Tool Identity And Aliases
The `pnpm` filter SHALL identify as `pnpm` and support platform aliases.

#### Scenario: alias executables
- **WHEN** executable is `pnpm.cmd`, `./pnpm.cmd`, `pnpm.exe`, or `./pnpm.exe`
- **THEN** the pnpm filter contract is used.

### Requirement: pnpm Command Scope
The `pnpm` phase SHALL provide dedicated handling for `list`, `outdated`, and `install` subcommands.

#### Scenario: supported subcommands
- **WHEN** subcommand is `list`, `outdated`, or `install`
- **THEN** pnpm-specific prepare/runtime behavior is applied.

#### Scenario: unsupported subcommand passthrough
- **WHEN** subcommand is outside supported scope
- **THEN** invocation remains passthrough.

### Requirement: Prepare Safety Rules
The filter SHALL use safety gates for structured modes and install package names.

#### Scenario: explicit structured list/outdated passthrough
- **WHEN** `list` uses explicit `--json`, or `outdated` uses explicit `--format json`
- **THEN** invocation is passthrough ambiguous.

#### Scenario: normalized list/outdated shape
- **WHEN** structured flags are not already explicit
- **THEN** `list` is normalized to include `--json` and default `--depth=0` (if missing), and `outdated` normalized to `--format json`.

#### Scenario: install package-name validation
- **WHEN** `pnpm install` package args include unsafe names
- **THEN** invocation is passthrough ambiguous with unsafe-package reason.

### Requirement: Runtime Event Model
The `pnpm` filter SHALL collect pre-exit events and decide output on exit.

#### Scenario: pre-exit collection
- **WHEN** event type is line, tick, or EOF
- **THEN** event is collected.

#### Scenario: no-output non-zero exit
- **WHEN** buffered output is empty and exit code is non-zero
- **THEN** no output is emitted.

### Requirement: Runtime Summary Behavior
The filter SHALL compact supported modes with bounded deterministic summaries.

#### Scenario: list summary
- **WHEN** list output parses via structured or degraded parser
- **THEN** output is `dependencies: <N>` with bounded package lines and optional `... +N more`.

#### Scenario: outdated summary
- **WHEN** outdated output parses
- **THEN** output is `outdated: <X>/<N>` plus bounded package version transitions.

#### Scenario: outdated empty success
- **WHEN** outdated mode exits successfully with no meaningful entries
- **THEN** output is `All packages up-to-date`.

#### Scenario: install summary and failure retention
- **WHEN** install output is compacted
- **THEN** progress noise is suppressed, failure lines retained, and summary lines deduped.
- **AND** empty successful install output normalizes to `ok`.

#### Scenario: empty compact-result fallback
- **WHEN** compaction returns empty output at exit
- **THEN** non-zero exits flush raw buffered output unchanged
- **AND** zero exits use mode-specific success markers where defined.

#### Scenario: low-confidence fallback
- **WHEN** output is low-confidence or parsing fails without degraded recovery
- **THEN** raw or bounded-truncated passthrough output is used.
