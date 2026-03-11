## Purpose
Define direct `golangci-lint` compaction behavior for supported default diagnostic invocation shapes.

## Requirements

### Requirement: Golangci-Lint Planning Safety
The `golangci-lint` phase SHALL compact only supported default diagnostic invocation shapes and keep explicit structured-output requests on passthrough.

#### Scenario: direct golangci-lint dispatch
- **WHEN** `golangci-lint` is invoked in a supported default `run`-style diagnostic mode
- **THEN** the phase uses the `golangci-lint` compactor.

#### Scenario: machine-readable passthrough
- **WHEN** `golangci-lint` is invoked with explicit machine-readable output modes
- **THEN** the phase returns passthrough behavior.

#### Scenario: supported default run shapes normalize to JSON
- **WHEN** a supported `golangci-lint` run shape does not explicitly request an output format
- **THEN** CCP may normalize the invocation to JSON for internal compaction.

### Requirement: Golangci-Lint Runtime Handling
The `golangci-lint` phase SHALL preserve bounded file, rule, and location signal while reducing repetitive diagnostics.

#### Scenario: grouped golangci-lint diagnostics
- **WHEN** supported `golangci-lint` issue output is compacted
- **THEN** output preserves file and bounded lint identity in a grouped line-oriented form.

#### Scenario: diagnostics and exit parity
- **WHEN** `golangci-lint` emits diagnostics or exits non-zero
- **THEN** actionable diagnostics remain visible and native exit-code parity is preserved.
