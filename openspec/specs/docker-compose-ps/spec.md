## Purpose
Define `docker compose ps` routing constraints and runtime compaction behavior.

## Requirements

### Requirement: Docker Compose Ps Planning Safety
The `docker compose ps` phase SHALL compact only safe listing-oriented invocation shapes.

#### Scenario: compose ps dispatch
- **WHEN** `docker compose ps` is invoked without explicit structured-output flags
- **THEN** the `docker` parent dispatches to the `docker compose ps` subfilter.

#### Scenario: explicit format passthrough
- **WHEN** `docker compose ps` args include `--format` or `--format=<value>`
- **THEN** the `docker compose ps` phase returns passthrough behavior.

### Requirement: Docker Compose Ps Runtime Compaction
The `docker compose ps` phase SHALL preserve service identity and high-signal status information while reducing listing noise.

#### Scenario: compact compose service list
- **WHEN** supported `docker compose ps` output is compacted
- **THEN** output preserves each service name and bounded status information in a line-oriented form.

#### Scenario: no synthetic summary line
- **WHEN** supported `docker compose ps` output is compacted
- **THEN** the compactor emits only service lines plus any overflow marker, not a synthetic top summary line.

#### Scenario: image references may be shortened
- **WHEN** a compose service image includes a registry or repository path
- **THEN** the compacted output may keep only the last path segment plus tag to reduce noise.

#### Scenario: stderr diagnostics remain visible
- **WHEN** `docker compose ps` emits diagnostics on stderr
- **THEN** those diagnostics remain visible to the caller.
