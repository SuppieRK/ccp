# init-claude-agent-integration Specification

## Purpose
Define Claude-specific init integration for deterministic hook/settings/awareness installation under the Claude home directory.

## Requirements
### Requirement: Claude Integration Targets
Claude init integration SHALL manage deterministic Claude files under the user Claude home directory.

#### Scenario: target paths are deterministic
- **WHEN** Claude integration is initialized
- **THEN** integration resolves and manages these paths:
  - `~/.claude/hooks/ccp-rewrite.sh`
  - `~/.claude/settings.json`
  - `~/.claude/CCP.md`

### Requirement: Claude PreToolUse Hook Installation
Claude init integration SHALL provision a runnable rewrite hook script and register it through Claude PreToolUse settings.

#### Scenario: hook materialization
- **WHEN** Claude integration is initialized
- **THEN** the rewrite hook script is created or updated at `~/.claude/hooks/ccp-rewrite.sh`.

#### Scenario: hook artifact is executable
- **WHEN** Claude integration writes the rewrite hook artifact
- **THEN** the installed hook file is set to executable permissions.

#### Scenario: PreToolUse registration is applied
- **WHEN** Claude integration is initialized
- **THEN** `~/.claude/settings.json` includes PreToolUse hook registration that points to the installed rewrite hook.

### Requirement: Claude Chained Command Rewrite Parity
Claude PreToolUse rewrite behavior SHALL preserve CCP routing across chained and piped shell command segments.

#### Scenario: chained command segments are all prefixed
- **WHEN** Claude PreToolUse receives a Bash command containing `&&`, `||`, `|`, or `;`
- **THEN** the rewrite output prefixes each command segment with `ccp`
- **AND** segments already starting with `ccp` are not double-prefixed.

#### Scenario: mixed prefixed and unprefixed chains are normalized
- **WHEN** Claude PreToolUse receives a command where only the first chain segment is prefixed with `ccp`
- **THEN** the rewrite output keeps the existing first segment
- **AND** prefixes remaining unprefixed chain segments with `ccp`.

### Requirement: Claude Rewrite Safety Fallback
Claude PreToolUse rewrite behavior SHALL fail safe to passthrough for command shapes that are not safely normalizable by the chain-prefix rewriter.

#### Scenario: complex quoting or substitution bypasses rewrite
- **WHEN** Claude PreToolUse receives a command containing complex quoting, shell substitution, escapes, or heredoc markers
- **THEN** the hook does not emit rewritten input
- **AND** command execution remains native passthrough.

#### Scenario: rewritten command must pass shell syntax check
- **WHEN** the hook computes a rewritten command string
- **THEN** the hook applies the rewritten command only if it passes shell syntax validation
- **AND** if syntax validation fails, the hook emits no rewrite and falls back to passthrough.

### Requirement: Automatic Settings Registration
Claude init integration SHALL materialize required PreToolUse settings automatically.

#### Scenario: settings file content is managed deterministically
- **WHEN** Claude integration writes `~/.claude/settings.json`
- **THEN** content includes the managed `PreToolUse` Bash command hook pointing at the installed rewrite script.

### Requirement: Claude Awareness Material Management
Claude init integration SHALL install awareness content with idempotent updates.

#### Scenario: awareness install
- **WHEN** Claude init executes
- **THEN** slim awareness content is provisioned/updated alongside hook installation.

#### Scenario: idempotent re-run
- **WHEN** Claude init is re-run without effective changes
- **THEN** managed artifacts are not rewritten and duplicate backups are not created.

### Requirement: Claude Uninstall Settings Cleanup
Claude uninstall integration SHALL remove only the managed Claude PreToolUse Bash command hook registration while preserving unrelated settings entries.

#### Scenario: uninstall removes managed PreToolUse entry
- **WHEN** Claude uninstall runs and `~/.claude/settings.json` contains a PreToolUse Bash command entry pointing to the managed hook path
- **THEN** that managed PreToolUse entry is removed from settings.

#### Scenario: uninstall preserves unrelated settings content
- **WHEN** Claude uninstall runs and `~/.claude/settings.json` contains unrelated hook entries or non-hook settings
- **THEN** unrelated settings content remains intact.

#### Scenario: uninstall tolerates malformed settings
- **WHEN** Claude uninstall runs and `~/.claude/settings.json` is malformed JSON
- **THEN** uninstall does not fail due to JSON parse errors
- **AND** stale malformed managed settings content is removed.
