## Purpose
Define `python` parent filter identity, prepare routing safety, and delegated runtime behavior.

## Requirements

### Requirement: Python Tool Identity And Aliases
The `python` parent filter SHALL identify as `python` and support interpreter alias executables.

#### Scenario: alias executables
- **WHEN** executable is `python3`, `python.exe`, `./python.exe`, `python3.exe`, `./python3.exe`, `python.cmd`, `./python.cmd`, `python3.cmd`, or `./python3.cmd`
- **THEN** the `python` parent filter contract is used.

### Requirement: Python Prepare Routing
The `python` parent SHALL default to passthrough and only delegate `-m pytest` invocations.

#### Scenario: script and non-module invocation passthrough
- **WHEN** invocation has args and no `-m <module>` routing target
- **THEN** args are preserved unchanged
- **AND** prepare remains passthrough.

#### Scenario: pytest module delegation
- **WHEN** invocation contains `-m pytest` (case-insensitive module match)
- **THEN** args after module selector are delegated to the pytest subfilter prepare behavior.
- **AND** dispatch key is `pytest`.
- **AND** normalized args preserve the original `-m pytest` prefix with delegated normalized suffix.

#### Scenario: non-pytest module passthrough
- **WHEN** invocation contains `-m <module>` where module is not `pytest`
- **THEN** parent remains passthrough with original args.

### Requirement: Interactive Invocation Safety
The parent SHALL treat REPL/interactive shapes as ambiguous passthrough.

#### Scenario: no args interactive invocation
- **WHEN** python is invoked without args
- **THEN** `ForcePassthrough=true`, `Ambiguous=true`, reason `interactive python invocation`.

#### Scenario: explicit interactive flags
- **WHEN** args include `-i` or `--interactive`
- **THEN** the same interactive ambiguous passthrough result is returned.

### Requirement: Parent Runtime Delegation
The parent SHALL delegate context/runtime by dispatch key and use noop fallback otherwise.

#### Scenario: pytest dispatch delegation
- **WHEN** dispatch equals `pytest`
- **THEN** `ContextKey(...)` and `Process(...)` are delegated to the pytest subfilter.

#### Scenario: unknown dispatch noop fallback
- **WHEN** dispatch is missing or does not resolve to `pytest`
- **THEN** parent falls back to noop behavior.
- **AND** line events are emitted immediately unchanged.
- **AND** tick, EOF, and exit events emit no output.
