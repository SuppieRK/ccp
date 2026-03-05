## Purpose
Define `docker` parent filter routing, safety boundaries, and delegated runtime behavior.

## Requirements

### Requirement: Docker Parent Routing
The `docker` phase SHALL route only supported subcommands to local compactors and keep unsupported/unsafe shapes passthrough.

#### Scenario: supported subcommand dispatch
- **WHEN** subcommand is `ps`, `images`, or `logs`
- **THEN** parent emits dispatch keys for local subfilters.

#### Scenario: structured output safety for ps/images
- **WHEN** `docker ps` or `docker images` args include `--format` (including `--format=<value>`)
- **THEN** parent returns ambiguous passthrough with structured-output reason.

#### Scenario: unsupported subcommand passthrough
- **WHEN** subcommand is outside supported routes
- **THEN** `ForcePassthrough=true`.

#### Scenario: compose passthrough boundary
- **WHEN** subcommand is `compose`
- **THEN** parent returns passthrough for safety.

#### Scenario: interactive or tty-heavy passthrough boundary
- **WHEN** subcommand is `exec`, `pull`, or `build`
- **THEN** parent returns passthrough with ambiguous/safety classification.

#### Scenario: no-args passthrough
- **WHEN** docker is invoked without args
- **THEN** parent returns passthrough.

### Requirement: Global-Flag-Aware Subcommand Detection
The `docker` parent SHALL skip recognized leading global flags before selecting subcommand behavior.

#### Scenario: leading global flags
- **WHEN** args begin with known global docker flags (`--context`, `--host/-H`, `--config`, `--log-level`, debug/tls flags)
- **THEN** those flags are moved aside for subcommand resolution.

### Requirement: Parent Runtime Delegation
The docker parent SHALL delegate runtime by dispatch key.

#### Scenario: logs dispatch normalization
- **WHEN** dispatch starts with `docker logs`
- **THEN** runtime resolves via `docker logs` subfilter.

#### Scenario: non-docker dispatch fallback
- **WHEN** dispatch is empty or does not start with `docker `
- **THEN** parent falls back to noop behavior.
