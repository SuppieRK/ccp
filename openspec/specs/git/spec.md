## Purpose
Define the parent `git` tool-filter contract: identity, subcommand routing, dispatch-key behavior, and safe fallback semantics.

## Requirements

### Requirement: Git Parent Tool Identity
The git parent filter SHALL register as top-level `git` with alias support.

#### Scenario: tool identity
- **WHEN** global tool registry resolves git commands
- **THEN** parent tool key is `git`.
- **AND** alias `git.exe` resolves to the same filter.

### Requirement: Git Local Subcommand Registry
The git parent filter SHALL own a local subcommand registry.

#### Scenario: supported subcommand registration
- **WHEN** git parent filter is constructed
- **THEN** handlers are registered for `status`, `diff`, `log`, `show`, `fetch`, `commit`, `push`, `pull`, `merge`, `rebase`, `blame`.

#### Scenario: longest-prefix subcommand match
- **WHEN** preparing git args for routing
- **THEN** parent attempts longest-prefix matching over up to two subcommand tokens.

### Requirement: Global-Flag Reordering For Routing
The git parent filter SHALL skip known leading global git flags before subcommand resolution.

#### Scenario: leading global flags
- **WHEN** args start with known global flags (for example `-C`, `-c`, `--git-dir`, `--work-tree`, `--namespace`, `--config-env`, `--exec-path`, pager/help/version flags)
- **THEN** those flags are moved aside for dispatch matching.
- **AND** consumed-token accounting keeps delegated argument slicing correct.
- **AND** `--git-dir=<v>`, `--work-tree=<v>`, `--namespace=<v>`, and `--config-env=<v>` are treated as single consumed tokens.

### Requirement: Prepare Delegation and Dispatch Key Assignment
The git parent filter SHALL delegate prepare behavior to resolved subcommand handlers.

#### Scenario: known subcommand delegation
- **WHEN** args resolve to a registered git subcommand
- **THEN** parent `Prepare(...)` delegates to that handler.
- **AND** `DispatchKey` is the resolved subcommand tool key.

#### Scenario: unknown or empty command passthrough
- **WHEN** args are empty or no registered subcommand matches
- **THEN** parent returns `ForcePassthrough=true`.

### Requirement: Dispatch-Based Runtime Delegation
The git parent filter SHALL route `ContextKey(...)` and `Process(...)` using `ev.Dispatch`.

#### Scenario: known git dispatch
- **WHEN** dispatch is a non-empty `git ...` key present in the local registry
- **THEN** runtime behavior is delegated to that subcommand filter.

#### Scenario: missing or unknown dispatch
- **WHEN** dispatch is empty, non-git-prefixed, or unresolved
- **THEN** parent falls back to noop behavior.

### Requirement: Parent Defaults
The git parent filter SHALL expose neutral parent defaults.

#### Scenario: masking horizon
- **WHEN** parent filter masking horizon is queried
- **THEN** it returns `0`.
