## Purpose
Define `docker logs` routing constraints, context isolation, and raw-preserving runtime behavior.

## Requirements

### Requirement: Docker Logs Parent Dispatch Safety
The `docker` parent route for `logs` SHALL be container-scoped and follow-mode safe.

#### Scenario: follow mode passthrough
- **WHEN** logs args include `-f` or `--follow`
- **THEN** parent returns ambiguous passthrough with follow-mode reason.

#### Scenario: non-follow container dispatch
- **WHEN** logs args resolve a container token and no follow flag
- **THEN** dispatch key is `docker logs|container=<container>`.
- **AND** container token resolution skips leading flag arguments, including long flags with `=` values and short/long flags that consume a following value (for example `-n`, `--tail`, `--since`, `--until`).

#### Scenario: missing container passthrough
- **WHEN** logs args do not resolve a container token
- **THEN** parent returns passthrough.

### Requirement: Docker Logs Runtime Handling
The `docker logs` subfilter SHALL preserve stderr immediacy and flush collected stdout without semantic rewriting.

#### Scenario: context isolation by container
- **WHEN** dispatch contains `container=<id-or-name>`
- **THEN** context key includes that container to prevent cross-container bleed.

#### Scenario: stderr preserved immediately
- **WHEN** stderr line events arrive
- **THEN** lines are emitted immediately unchanged.

#### Scenario: stdout collection and flush
- **WHEN** stdout line/tick events arrive
- **THEN** events are collected.
- **AND** at EOF/Exit, non-empty buffered stdout is flushed unchanged.

#### Scenario: empty stdout
- **WHEN** buffered stdout is empty at EOF/Exit
- **THEN** no output is emitted.
