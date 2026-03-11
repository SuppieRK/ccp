## Purpose
Define direct `tsc` planning safety and runtime diagnostic compaction behavior.

## Requirements

### Requirement: Tsc Planning Safety
The direct `tsc` phase SHALL compact only supported human-readable diagnostic invocation shapes.

#### Scenario: direct tsc dispatch
- **WHEN** `tsc` is invoked in a supported diagnostic-oriented mode
- **THEN** the phase uses the direct `tsc` compactor.

#### Scenario: pretty-enabled passthrough
- **WHEN** the direct `tsc` invocation explicitly enables pretty output
- **THEN** the phase returns passthrough behavior.

#### Scenario: unsupported shape passthrough
- **WHEN** the direct `tsc` invocation shape is unsupported or precision-sensitive
- **THEN** the phase returns passthrough behavior.

### Requirement: Tsc Runtime Handling
The direct `tsc` phase SHALL preserve bounded file, code, and location signal while reducing repetitive diagnostics.

#### Scenario: grouped tsc diagnostics
- **WHEN** supported direct `tsc` output is compacted
- **THEN** output preserves file and bounded compiler diagnostic identity in a grouped line-oriented form.

#### Scenario: diagnostics and exit parity
- **WHEN** direct `tsc` emits diagnostics or exits non-zero
- **THEN** actionable diagnostics remain visible and native exit-code parity is preserved.

#### Scenario: parse fallback
- **WHEN** direct `tsc` output does not match parseable diagnostic lines
- **THEN** raw buffered output is flushed unchanged.
