## Purpose
Define `docker compose logs` routing constraints, context isolation, and buffered runtime behavior.

## Requirements

### Requirement: Docker Compose Logs Planning Safety
The `docker compose logs` phase SHALL route only non-streaming log shapes through a dedicated subfilter.

#### Scenario: compose logs dispatch
- **WHEN** `docker compose logs` is invoked without follow-mode flags
- **THEN** the `docker` parent dispatches to the `docker compose logs` subfilter.

#### Scenario: follow mode passthrough
- **WHEN** `docker compose logs` args include follow-mode behavior
- **THEN** the `docker` parent returns ambiguous passthrough with follow-mode reason.

### Requirement: Docker Compose Logs Runtime Handling
The `docker compose logs` phase SHALL preserve service identity while using buffered log runtime behavior.

#### Scenario: context isolation by service scope
- **WHEN** dispatch contains `scope=<service-list-or-all>`
- **THEN** context key includes that scope to prevent cross-service bleed.

#### Scenario: stderr preserved immediately
- **WHEN** stderr line events arrive
- **THEN** lines are emitted immediately unchanged.

#### Scenario: stdout collection and flush
- **WHEN** stdout line/tick events arrive
- **THEN** events are collected.
- **AND** at EOF/Exit, non-empty buffered stdout is flushed through the shared buffered log handler.

#### Scenario: empty stdout
- **WHEN** buffered stdout is empty at EOF/Exit
- **THEN** no output is emitted.
