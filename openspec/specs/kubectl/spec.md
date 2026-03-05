## Purpose
Define `kubectl` parent filter routing and safety behavior.

## Requirements

### Requirement: Kubectl Tool Identity
The parent tool SHALL identify as `kubectl` and support `kubectl.exe` alias resolution.

#### Scenario: executable alias
- **WHEN** command executable is `kubectl.exe`
- **THEN** the `kubectl` parent filter contract is used.

### Requirement: Kubectl Parent Routing
The `kubectl` parent SHALL route only allowlisted subcommands and passthrough others.

#### Scenario: routed commands
- **WHEN** args resolve to `get pods|pod`, `get nodes|node`, `get services|service|svc`, or non-follow `logs`
- **THEN** parent emits dispatch key `kubectl get pods`, `kubectl get nodes`, `kubectl get services`, or `kubectl logs` respectively.

#### Scenario: unsupported commands
- **WHEN** args do not resolve to one of the routed commands
- **THEN** parent returns passthrough.

#### Scenario: no-args passthrough
- **WHEN** `kubectl` is invoked without args
- **THEN** parent returns passthrough.

### Requirement: Structured Output Safety
The `kubectl` parent SHALL passthrough explicit structured output modes.

#### Scenario: structured output flags
- **WHEN** args include `-o`/`--output` set to `yaml`, `json`, `jsonpath...`, or `name`
- **THEN** parent returns passthrough.

### Requirement: Global-Flag-Aware Routing
The parent SHALL skip recognized leading global kubectl flags before subcommand matching.

#### Scenario: known leading globals
- **WHEN** args begin with global flags (`-n`/`--namespace`, `--context`, `--cluster`, `--user`, `--server`, including `=<value>` forms)
- **THEN** routing ignores those leading flag prefixes.
- **AND** delegated subcommand-argument evaluation preserves those global flags after routed subcommand tokens.

### Requirement: Logs Follow Safety
The parent SHALL not route follow-mode logs to semantic compaction.

#### Scenario: follow-mode logs passthrough
- **WHEN** `kubectl logs` args include `-f` or `--follow`
- **THEN** parent returns passthrough.
