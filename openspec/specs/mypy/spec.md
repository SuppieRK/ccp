## ADDED Requirements

### Requirement: Mypy Planning Safety
The `mypy` phase SHALL compact only supported human-readable diagnostic invocation shapes.

#### Scenario: direct mypy dispatch
- **WHEN** `mypy` is invoked in a supported diagnostic-oriented mode
- **THEN** the phase uses the mypy compactor.

#### Scenario: machine-readable passthrough
- **WHEN** `mypy` is invoked with explicit machine-readable output modes
- **THEN** the phase returns passthrough behavior.

### Requirement: Mypy Runtime Handling
The `mypy` phase SHALL preserve bounded file, location, and error signal while reducing repetitive diagnostics.

#### Scenario: grouped mypy diagnostics
- **WHEN** supported mypy output is compacted
- **THEN** output preserves file and bounded diagnostic identity in a grouped line-oriented form.

#### Scenario: diagnostics and exit parity
- **WHEN** mypy emits diagnostics or exits non-zero
- **THEN** actionable diagnostics remain visible and native exit-code parity is preserved.
