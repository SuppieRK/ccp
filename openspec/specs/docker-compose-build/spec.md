## ADDED Requirements

### Requirement: Docker Compose Build Planning Safety
The `docker compose build` phase SHALL route only supported build-oriented compose shapes through a dedicated compactor.

#### Scenario: compose build dispatch
- **WHEN** `docker compose build` is invoked in a supported non-interactive shape
- **THEN** the `docker` parent dispatches to the `docker compose build` subfilter.

#### Scenario: unsupported compose passthrough
- **WHEN** the compose invocation shape is outside supported `compose build` handling
- **THEN** the phase returns passthrough behavior.

### Requirement: Docker Compose Build Runtime Handling
The `docker compose build` phase SHALL preserve service identity and actionable failure signal while reducing repetitive progress output.

#### Scenario: bounded build summary
- **WHEN** supported `docker compose build` output is compacted
- **THEN** output includes bounded per-service build completion or in-progress information derived from service-level `Built` lines and BuildKit step markers.

#### Scenario: low-confidence fallback
- **WHEN** build output cannot be compacted with sufficient confidence
- **THEN** the phase falls back to passthrough output.

#### Scenario: diagnostics and exit parity
- **WHEN** `docker compose build` emits diagnostics or exits non-zero
- **THEN** actionable diagnostics remain visible and native exit-code parity is preserved.
