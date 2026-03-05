## Purpose
Define execution-path command proxy behavior for `ccp` command wrapping, planning, and shared tool registration.
## Requirements
### Requirement: Execution Command Wrapping
`ccp` SHALL execute wrapped commands and preserve process exit behavior.

#### Scenario: Exit-code parity
- **WHEN** a wrapped command exits with code `N`
- **THEN** `ccp` exits with code `N`.

#### Scenario: Missing execution command
- **WHEN** no execution command is provided (and no lifecycle subcommand is selected)
- **THEN** `ccp` prints usage and exits with non-zero status.

#### Scenario: bounded line ingestion preserves truncation marker behavior
- **WHEN** wrapped stream lines exceed runner line-size bounds
- **THEN** runner preserves bounded-line truncation behavior and continues deterministic stream processing.

### Requirement: Execution Flags
`ccp` SHALL support execution-mode flags `--raw`, `--strict`, and `--debug-filter`.

#### Scenario: Default compacted mode
- **WHEN** `ccp` runs an execution command without `--raw`
- **THEN** output is processed through the semantic engine path.

#### Scenario: Raw bypass
- **WHEN** `ccp` runs with `--raw`
- **THEN** runner output is passed through without invoking engine filtering/compaction logic.

#### Scenario: Raw flag scope
- **WHEN** `--raw` is used with lifecycle subcommands (`init`, `gain`, `history`, `upgrade`, `uninstall`)
- **THEN** argument parsing fails with an error indicating `--raw` is execution-only.

#### Scenario: Debug annotation
- **WHEN** `--debug-filter` is enabled and filtered output is emitted
- **THEN** debug metadata is emitted on stderr with sequence/key/action context.

### Requirement: Ambiguity Handling
Execution planning SHALL detect ambiguous shell-chain operators and apply strict/permissive policy.

#### Scenario: Strict rejection
- **WHEN** planning detects `&&`, `||`, `;`, or `|` and `--strict` is enabled
- **THEN** planning fails pre-flight and execution does not start.

#### Scenario: Strict mode requires single-tool semantic context
- **WHEN** `--strict` is enabled and planner cannot guarantee a single-tool semantic context
- **THEN** execution is refused pre-flight with a clear ambiguity diagnostic.

#### Scenario: Permissive fallback
- **WHEN** planning detects shell-chain operators and strict mode is disabled
- **THEN** planning executes through shell fallback.

#### Scenario: Permissive fallback uses neutral filtering
- **WHEN** planning detects shell-chain operators and strict mode is disabled
- **THEN** execution proceeds without tool-specific semantic compaction
- **AND** output processing uses a neutral/noop path to avoid cross-tool miscompaction.

### Requirement: Shared Tool-Aware Planning
Execution planning SHALL use the same registry-backed tool contracts used by the engine.

#### Scenario: Tool preparation from registry
- **WHEN** a detected tool has a registered filter
- **THEN** planner applies `Prepare(...)` from that tool contract to normalize args.

#### Scenario: Unknown-tool neutral behavior
- **WHEN** no registered filter matches a detected tool
- **THEN** planner and engine both use tool-scoped no-op behavior instead of divergent tool logic.

#### Scenario: Strict grep no-match tagging
- **WHEN** planning a `grep` command with `--strict` enabled
- **THEN** planner includes `strict_no_match=1` in dispatch metadata to enforce strict no-match runtime semantics.

### Requirement: Explicit Registry Construction
The tool filter registry SHALL be constructed explicitly in startup wiring and shared across planner and engine.

#### Scenario: Explicit registration
- **WHEN** runtime dependencies are built
- **THEN** filters are registered through explicit `Register(...)` calls in startup code.

#### Scenario: Duplicate registration failure
- **WHEN** multiple filters register the same canonical tool or alias
- **THEN** startup fails with a registration error.

### Requirement: Tool-Owned Subcommand Dispatch
Subcommand routing SHALL be owned by tool filters rather than global registry expansion.

#### Scenario: global registry resolves top-level tools
- **WHEN** planner or engine resolves a command filter
- **THEN** the global `ToolFilterRegistry` resolves only top-level tools (for example `git`)
- **AND** does not require global registrations for every subcommand key.

#### Scenario: tool-local subcommand resolution
- **WHEN** a resolved top-level tool supports subcommands
- **THEN** that tool filter resolves and delegates subcommands using its own internal routing/registry model.

#### Scenario: tool-local unknown subcommand fallback
- **WHEN** a tool-local subcommand resolver cannot map command shape to a registered handler
- **THEN** execution falls back to parent-tool safe behavior (typically passthrough).

### Requirement: Explicit Dispatch Key Propagation
Routing metadata SHALL be propagated explicitly from planning to runtime events.

#### Scenario: planner emits dispatch key
- **WHEN** planner resolves a tool/subcommand handler
- **THEN** `ExecPlan` includes a stable `DispatchKey` identifying the resolved handler.

#### Scenario: runner forwards dispatch key
- **WHEN** runner forwards command output to the semantic engine
- **THEN** it includes `DispatchKey` on line, EOF, exit, and tick-context engine inputs.

#### Scenario: filters route using dispatch key
- **WHEN** a tool filter receives events with a known `DispatchKey`
- **THEN** it resolves internal handler routing from that key
- **AND** it does not rely on parsing `CommandID` for routing correctness.
- **AND** parent-tool delegation fallback decisions are evaluated from event-context routing inputs.

### Requirement: Execution Metrics Recording Across Proxy Outcomes
The command proxy SHALL record one metrics entry for each completed non-raw execution command regardless of whether output was compacted or passed through.

#### Scenario: Compacted proxy path records metrics
- **WHEN** an execution command is handled by proxy filtering (non-passthrough)
- **THEN** the proxy records a metrics entry with raw and kept byte measurements and marks passthrough as false.

#### Scenario: Passthrough proxy path records metrics
- **WHEN** an execution command is passed through within proxy execution flow
- **THEN** the proxy records a metrics entry with byte measurements and marks passthrough as true.

#### Scenario: Raw mode remains excluded from gain tracking
- **WHEN** an execution command runs with `--raw`
- **THEN** the proxy does not append gain metrics for that run.

#### Scenario: Metrics write failure does not change command outcome
- **WHEN** metrics persistence fails after command execution
- **THEN** command exit code and emitted output remain unchanged.
