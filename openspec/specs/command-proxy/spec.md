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
`ccp` SHALL support execution-mode flags `--raw`, `--debug-filter`, and `--confidential`.

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

#### Scenario: Confidential flag is execution-scoped
- **WHEN** `--confidential` is used with an execution command
- **THEN** parsing succeeds without requiring `--capture-raw`.

#### Scenario: Confidential flag remains invalid for lifecycle subcommands
- **WHEN** `--confidential` is used with lifecycle subcommands (`init`, `gain`, `history`, `upgrade`, `uninstall`)
- **THEN** argument parsing fails with an error indicating the flag is execution-only.

### Requirement: Ambiguity Handling
Execution planning SHALL detect ambiguous shell-chain operators and preserve safety through permissive fallback behavior.

#### Scenario: Permissive fallback
- **WHEN** planning detects `&&`, `||`, `;`, or `|`
- **THEN** planning executes through shell fallback.

#### Scenario: Permissive fallback uses neutral filtering
- **WHEN** planning detects shell-chain operators and falls back
- **THEN** execution proceeds without tool-specific semantic compaction
- **AND** output processing uses a neutral/noop path to avoid cross-tool miscompaction.

#### Scenario: prepare ambiguity uses neutral fallback
- **WHEN** planner cannot guarantee a single-tool semantic context from tool preparation
- **THEN** execution proceeds through neutral/passthrough-safe behavior
- **AND** execution is not rejected solely because the shape is ambiguous.

### Requirement: Shared Tool-Aware Planning
Execution planning SHALL use the same registry-backed tool contracts used by the engine.

#### Scenario: Tool preparation from registry
- **WHEN** a detected tool has a registered filter
- **THEN** planner applies `Prepare(...)` from that tool contract to normalize args.

#### Scenario: recognized passthrough retains tool identity
- **WHEN** a detected tool has a registered filter
- **AND** that tool contract selects passthrough-safe execution for the command shape
- **THEN** the execution plan retains the canonical tool identity for metrics and history classification
- **AND** passthrough behavior is represented through the passthrough execution path rather than by clearing the tool identity

#### Scenario: Unknown-tool neutral behavior
- **WHEN** no registered filter matches a detected tool
- **THEN** planner and engine both use tool-scoped no-op behavior instead of divergent tool logic.

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

#### Scenario: recognized passthrough preserves canonical tool classification
- **WHEN** an execution command is passed through within proxy execution flow
- **AND** the command was recognized by a registered tool contract
- **THEN** the proxy records the canonical tool identity for that command in metrics and history outputs.

#### Scenario: ambiguous shell fallback remains neutral
- **WHEN** planning falls back to neutral shell execution because command shape is ambiguous
- **THEN** the proxy may persist a neutral or unknown tool classification for that execution
- **AND** does not claim a specific canonical tool identity solely from the shell-chain contents.

#### Scenario: Raw mode remains excluded from gain tracking
- **WHEN** an execution command runs with `--raw`
- **THEN** the proxy does not append gain metrics for that run.

#### Scenario: Metrics write failure does not change command outcome
- **WHEN** metrics persistence fails after command execution
- **THEN** command exit code and emitted output remain unchanged.

### Requirement: Execution command stdin propagation
The command proxy SHALL propagate stdin from the `ccp` process into wrapped command execution for both semantic and raw paths.

#### Scenario: Semantic mode forwards piped stdin
- **WHEN** `ccp` runs a stdin-driven command in semantic mode and stdin contains bytes
- **THEN** the wrapped command receives the same stdin byte stream
- **AND** emitted stdout/stderr reflects normal execution against that input.

#### Scenario: Raw mode forwards piped stdin
- **WHEN** `ccp --raw` runs a stdin-driven command and stdin contains bytes
- **THEN** the wrapped command receives the same stdin byte stream
- **AND** `ccp` preserves raw passthrough semantics for resulting output and exit code.

### Requirement: Confidential Output Redaction
The command proxy SHALL redact configured confidential substrings from emitted execution output.

#### Scenario: Semantic mode redacts emitted output
- **WHEN** `ccp --confidential a,b <command>` emits stdout or stderr in semantic mode
- **THEN** emitted output replaces each configured substring with `***`.

#### Scenario: Raw mode redacts emitted output
- **WHEN** `ccp --raw --confidential a,b <command>` emits stdout or stderr
- **THEN** emitted output replaces each configured substring with `***`.

#### Scenario: Capture mode also redacts capture files
- **WHEN** `ccp --capture-raw --confidential a,b <command>` writes capture artifacts
- **THEN** the capture files redact the same configured substrings.

### Requirement: Stdin-driven pipeline parity
The command proxy SHALL preserve native observable behavior for stdin-driven pipeline scenarios.

#### Scenario: Pipeline producer output is consumable by wrapped command
- **WHEN** a producer command pipes output into `ccp <consumer-command>` where the consumer reads stdin
- **THEN** proxied consumer output SHALL match native consumer behavior for equivalent input
- **AND** proxied consumer exit code SHALL match native consumer exit code.

#### Scenario: Empty producer input remains empty
- **WHEN** a producer sends zero bytes to `ccp <consumer-command>`
- **THEN** the wrapped consumer receives zero bytes
- **AND** `ccp` emits zero bytes unless the native consumer would emit output for empty input.

### Requirement: Stdin diagnostics metadata tagging
The command proxy SHALL encode stdin-presence diagnostics in execution dispatch metadata using the existing dispatch-tag pattern.

#### Scenario: Metrics/history include stdin mode via dispatch
- **WHEN** a command runs with stdin connected to a pipe, terminal, or closed/empty source
- **THEN** execution dispatch metadata includes a stable stdin mode marker (for example `stdin=pipe`, `stdin=tty`, or `stdin=none`)
- **AND** persisted metrics/history diagnostics expose that marker through the existing dispatch field.

### Requirement: Safe handling for stdin-sensitive ambiguous contexts
When planner confidence is insufficient for stdin-sensitive command contexts, the command proxy SHALL preserve safety by avoiding tool-specific miscompaction.

#### Scenario: Ambiguous stdin-sensitive context under permissive mode
- **WHEN** stdin is present and command planning falls back to ambiguous permissive execution
- **THEN** execution proceeds via neutral/passthrough-safe filtering behavior
- **AND** input delivery and exit semantics remain consistent with native execution.
