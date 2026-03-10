# init-qwen-agent-integration Specification

## Purpose
Define how `ccp init` detects, installs, verifies, and uninstalls the Qwen Code integration.

## Requirements
### Requirement: Detect Qwen Projects by `.qwen`

`ccp init` MUST detect Qwen Code projects using the repo-local `.qwen` directory heuristic.

#### Scenario: detect Qwen from project marker
- **WHEN** the current repository contains a `.qwen` directory
- **THEN** `qwen` is included in detected init candidates

### Requirement: Qwen Uses User-Scoped Configuration Or Context
Qwen init integration SHALL install CCP through the user-scoped Qwen settings and user context layer rather than through repo-root `AGENTS.md`.

#### Scenario: install managed qwen user contribution
- **WHEN** the user runs `ccp init --tools qwen`
- **THEN** CCP sets `context.fileName` to `AGENTS.md` in `~/.qwen/settings.json`
- **AND** CCP installs the managed user context file at `~/.qwen/AGENTS.md`
- **AND** repository `.qwen` markers remain detection inputs rather than the install target

### Requirement: Qwen Uninstall Preserves Unrelated User Configuration
Qwen uninstall integration SHALL remove only the CCP-managed Qwen user-scoped contribution.

#### Scenario: uninstall preserves unrelated qwen config
- **WHEN** the user uninstalls Qwen integration
- **THEN** CCP removes only the CCP-managed `context.fileName` contribution and the managed `~/.qwen/AGENTS.md` file
- **AND** preserves unrelated Qwen settings and context files
