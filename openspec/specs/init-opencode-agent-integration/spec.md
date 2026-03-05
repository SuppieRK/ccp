# init-opencode-agent-integration Specification

## Purpose
Define OpenCode-specific init integration using OpenCode plugin hooks for command rewrite through `ccp`.

## Requirements

### Requirement: OpenCode Target Resolution
OpenCode init adapter SHALL resolve a deterministic plugin target path from init scope.

#### Scenario: local scope target
- **WHEN** a user runs `ccp init --tools opencode` in a repository
- **THEN** adapter targets `.opencode/plugins/ccp-rewrite.js` under repository scope root.

#### Scenario: global scope target
- **WHEN** a user runs `ccp init --global --tools opencode`
- **THEN** adapter targets `~/.config/opencode/plugins/ccp-rewrite.js`.

### Requirement: OpenCode Tool Interception Wiring
`ccp init --tools opencode` SHALL write/update OpenCode plugin integration so bash command execution is routed through `ccp`.

#### Scenario: OpenCode wiring applied
- **WHEN** user runs `ccp init --tools opencode`
- **THEN** adapter installs/updates a plugin file in OpenCode plugin directory
- **AND** plugin implements `tool.execute.before`
- **AND** plugin rewrites only bash tool commands to execute via `ccp`.

### Requirement: Bash-Only Rewrite Scope
OpenCode rewrite integration SHALL apply only to bash tool execution and SHALL NOT alter non-bash tools.

#### Scenario: Non-bash tool execution
- **WHEN** OpenCode executes a tool other than `bash`
- **THEN** CCP rewrite plugin makes no command mutation.

### Requirement: No Double Prefixing
OpenCode rewrite integration SHALL avoid rewrapping commands that already invoke `ccp`.

#### Scenario: Already wrapped command
- **WHEN** a bash tool command already starts with `ccp `
- **THEN** plugin leaves the command unchanged.

#### Scenario: Bare ccp command
- **WHEN** a bash tool command is exactly `ccp` (after leading-trim normalization)
- **THEN** plugin leaves the command unchanged.

### Requirement: OpenCode Idempotent Reapply
OpenCode adapter SHALL be idempotent on repeated runs.

#### Scenario: Re-run opencode init
- **WHEN** `ccp init --tools opencode` is run twice
- **THEN** second run reports no-op or already configured state
- **AND** does not duplicate plugin files or duplicate rewrite logic.
