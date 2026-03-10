## MODIFIED Requirements

### Requirement: Docker Parent Routing
The `docker` phase SHALL route only supported subcommands to local compactors and keep unsupported/unsafe shapes passthrough.

#### Scenario: supported subcommand dispatch
- **WHEN** subcommand is `ps`, `images`, `logs`, or `compose build`
- **THEN** parent emits dispatch keys for local subfilters.

#### Scenario: structured output safety for ps/images
- **WHEN** `docker ps` or `docker images` args include `--format` (including `--format=<value>`)
- **THEN** parent returns ambiguous passthrough with structured-output reason.

#### Scenario: unsupported subcommand passthrough
- **WHEN** subcommand is outside supported routes
- **THEN** `ForcePassthrough=true`.

#### Scenario: unsupported compose passthrough boundary
- **WHEN** subcommand is `compose` with a nested subcommand outside supported routes
- **THEN** parent returns passthrough for safety.

#### Scenario: interactive or tty-heavy passthrough boundary
- **WHEN** subcommand is `exec`, `pull`, or `build`
- **THEN** parent returns passthrough with ambiguous/safety classification.

#### Scenario: no-args passthrough
- **WHEN** docker is invoked without args
- **THEN** parent returns passthrough.
