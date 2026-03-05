## Purpose
Define `npx` parent filter routing and delegation behavior.

## Requirements

### Requirement: npx Tool Identity And Aliases
The `npx` parent filter SHALL identify as `npx` and support platform aliases.

#### Scenario: alias executables
- **WHEN** executable is `npx.cmd`, `./npx.cmd`, `npx.exe`, or `./npx.exe`
- **THEN** the `npx` parent filter contract is used.

### Requirement: npx Parent Dispatch
The `npx` phase SHALL act as a parent dispatcher and route supported first executable-token tools.

#### Scenario: supported tool route
- **WHEN** first non-flag token resolves to `tsc|typescript`, `eslint`, `prettier`, `prisma`, or `node`
- **THEN** the matching dispatch key (`npx tsc`, `npx eslint`, `npx prettier`, `npx prisma`, `npx node`) is emitted.

#### Scenario: unsupported tool route
- **WHEN** first resolved tool token is not in the allowlist
- **THEN** parent stays passthrough.

#### Scenario: leading flags and delimiter
- **WHEN** invocation includes leading flags and optional `--` delimiter before executable token
- **THEN** dispatch resolution skips those prefixes and resolves the first executable token after them.

### Requirement: Conservative Routing Safety
The `npx` parent SHALL fail safe to passthrough for low-confidence invocation shapes.

#### Scenario: package-injection flags
- **WHEN** args include `-p/--package` (or `=...` forms)
- **THEN** routing is treated as low-confidence and passthrough is used.

#### Scenario: empty or malformed invocation
- **WHEN** no executable token can be resolved after flags
- **THEN** parent uses passthrough.

### Requirement: Parent Runtime Delegation
The parent SHALL delegate runtime behavior by `ev.Dispatch` for `npx ...` keys only.

#### Scenario: known dispatch
- **WHEN** dispatch starts with `npx ` and resolves in registry
- **THEN** `ContextKey(...)` and `Process(...)` are delegated to the subfilter.

#### Scenario: unknown dispatch
- **WHEN** dispatch is empty, non-`npx` prefixed, or unresolved
- **THEN** parent falls back to noop behavior.
