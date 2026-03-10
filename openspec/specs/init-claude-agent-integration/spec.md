# init-claude-agent-integration Specification

## Purpose
Define Claude-specific init integration for deterministic hook/settings/awareness installation under the Claude home directory.

## Requirements
### Requirement: Claude Integration Targets
Claude init integration SHALL manage deterministic Claude hook, settings, awareness, and guidance files under the user Claude home directory.

#### Scenario: target paths are deterministic
- **WHEN** Claude integration is initialized
- **THEN** integration resolves and manages these paths:
  - `~/.claude/hooks/ccp-rewrite.sh`
  - `~/.claude/settings.json`
  - `~/.claude/CCP.md`
  - `~/.claude/CLAUDE.md`

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

#### Scenario: generated hook is self-contained
- **WHEN** Claude PreToolUse executes the managed rewrite hook
- **THEN** rewrite behavior does not depend on `jq` being installed on the user machine.

#### Scenario: ordinary quoted commands are still rewritten
- **WHEN** Claude PreToolUse receives a Bash command that uses ordinary single or double quotes without escapes, substitutions, or heredoc syntax
- **THEN** the hook still prefixes the command or chained segments with `ccp`
- **AND** does not classify quoting alone as a rewrite blocker.

#### Scenario: command text is not rejected by heuristic complexity checks
- **WHEN** Claude PreToolUse receives valid Bash command text including quotes, backslashes, substitutions, or heredoc tokens
- **THEN** the hook still attempts the `ccp` prefix rewrite
- **AND** relies on rewrite/no-op detection and shell syntax validation instead of a pre-rewrite complexity screen.

### Requirement: Claude Rewrite Safety Fallback
Claude PreToolUse rewrite behavior SHALL fail safe to passthrough for command shapes that are not safely normalizable by the chain-prefix rewriter.

#### Scenario: rewritten command must pass shell syntax check
- **WHEN** the hook computes a rewritten command string
- **THEN** the hook applies the rewritten command only if it passes shell syntax validation
- **AND** if syntax validation fails, the hook emits no rewrite and falls back to passthrough.

#### Scenario: skip paths leave troubleshooting markers
- **WHEN** the managed Claude hook exits early without rewriting
- **THEN** it appends a deterministic reason marker to a log file under the system tmp directory
- **AND** each early-return branch uses a distinct marker so users can identify why rewrite was skipped.

### Requirement: Automatic Settings Registration
Claude init integration SHALL materialize required PreToolUse settings automatically.

#### Scenario: settings file content is managed deterministically
- **WHEN** Claude integration writes `~/.claude/settings.json`
- **THEN** content includes the managed `PreToolUse` Bash command hook pointing at the installed rewrite script.

### Requirement: Claude Awareness Material Management
Claude init integration SHALL install awareness content with idempotent updates.

#### Scenario: awareness install
- **WHEN** Claude init executes
- **THEN** slim awareness content is provisioned or updated at `~/.claude/CCP.md`
- **AND** `~/.claude/CLAUDE.md` contains a managed CCP reference block that points Claude at `CCP.md`.

#### Scenario: awareness preserves unrelated global instructions
- **WHEN** Claude init executes and `~/.claude/CLAUDE.md` already contains user-authored content
- **THEN** Claude integration updates or inserts only the CCP-managed block
- **AND** preserves unrelated content outside that managed block.

#### Scenario: idempotent re-run
- **WHEN** Claude init is re-run without effective changes
- **THEN** managed artifacts are not rewritten and duplicate managed guidance blocks are not created.

### Requirement: Claude Uninstall Settings Cleanup
Claude uninstall integration SHALL remove only the managed Claude PreToolUse Bash command hook registration while preserving unrelated settings entries.

#### Scenario: uninstall removes managed PreToolUse entry
- **WHEN** Claude uninstall runs and `~/.claude/settings.json` contains a PreToolUse Bash command entry pointing to the managed hook path
- **THEN** that managed PreToolUse entry is removed from settings.

#### Scenario: uninstall preserves unrelated settings content
- **WHEN** Claude uninstall runs and `~/.claude/settings.json` contains unrelated hook entries or non-hook settings
- **THEN** unrelated settings content remains intact.

#### Scenario: uninstall removes only managed Claude guidance block
- **WHEN** Claude uninstall runs and `~/.claude/CLAUDE.md` contains both unrelated user content and the managed CCP reference block
- **THEN** uninstall removes only the managed CCP block
- **AND** preserves unrelated global Claude instructions.

#### Scenario: uninstall cleans up fully managed Claude guidance file
- **WHEN** Claude uninstall runs and `~/.claude/CLAUDE.md` contains only the managed CCP reference block
- **THEN** uninstall removes the now-empty `CLAUDE.md` file.

#### Scenario: uninstall tolerates malformed settings
- **WHEN** Claude uninstall runs and `~/.claude/settings.json` is malformed JSON
- **THEN** uninstall does not fail due to JSON parse errors
- **AND** stale malformed managed settings content is removed.
