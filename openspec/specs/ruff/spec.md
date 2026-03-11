## Purpose
Define direct `ruff` planning safety and runtime diagnostic compaction behavior.

## Requirements

### Requirement: Ruff Planning Safety
The `ruff` phase SHALL compact only supported human-readable diagnostic invocation shapes.

#### Scenario: direct ruff dispatch
- **WHEN** `ruff` is invoked in a supported diagnostic-oriented mode
- **THEN** the phase uses the Ruff compactor.

#### Scenario: machine-readable passthrough
- **WHEN** `ruff` is invoked with explicit machine-readable output modes
- **THEN** the phase returns passthrough behavior.

#### Scenario: unsupported shape passthrough
- **WHEN** `ruff` is invoked with unsupported or precision-sensitive shapes
- **THEN** the phase returns passthrough behavior.

### Requirement: Ruff Runtime Handling
The `ruff` phase SHALL preserve bounded file, rule, and location signal while reducing repetitive diagnostic output.

#### Scenario: grouped ruff diagnostics
- **WHEN** supported Ruff output is compacted
- **THEN** output preserves file and bounded diagnostic identity in a grouped line-oriented form.

#### Scenario: diagnostics and exit parity
- **WHEN** Ruff emits diagnostics or exits non-zero
- **THEN** actionable diagnostics remain visible and native exit-code parity is preserved.

#### Scenario: parse fallback
- **WHEN** Ruff output does not match parseable diagnostic lines
- **THEN** raw buffered output is flushed unchanged.
