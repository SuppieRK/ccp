## Purpose
Define `go` parent filter routing and safety behavior.

## Requirements

### Requirement: Go Parent Routing
The `go` phase SHALL route only `go test` and `go build` to specialized filters and passthrough other subcommands.

#### Scenario: supported subcommand dispatch
- **WHEN** subcommand is `test` or `build`
- **THEN** dispatch key is `go <subcommand>`.

#### Scenario: go build -x dispatch
- **WHEN** subcommand is `build` and args include `-x`
- **THEN** dispatch key is `go build|x=1`.

#### Scenario: unknown subcommand passthrough
- **WHEN** subcommand is not `test` or `build`
- **THEN** parent returns passthrough.

#### Scenario: no-args passthrough
- **WHEN** `go` is invoked without args
- **THEN** parent returns passthrough.

### Requirement: Global-Flag-Aware Routing
The `go` parent SHALL skip known leading global flags while resolving the routed subcommand.

#### Scenario: leading `-C` global flag
- **WHEN** args begin with `-C` or `-C=<dir>`
- **THEN** subcommand matching ignores the global flag prefix.

### Requirement: Parent Structured Output Safety
The `go` parent SHALL passthrough structured mode for `go test`.

#### Scenario: go test json mode in parent
- **WHEN** `go test` args include `-json`
- **THEN** parent marks passthrough with ambiguous structured-output reason.
