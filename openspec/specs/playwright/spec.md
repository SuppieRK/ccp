## Purpose
Define direct `playwright` planning safety and runtime test-summary compaction behavior.

## Requirements

### Requirement: Playwright Planning Safety
The `playwright` phase SHALL compact only supported human-readable invocation shapes.

#### Scenario: direct playwright dispatch
- **WHEN** `playwright` is invoked in a supported default reporting mode
- **THEN** the phase uses the Playwright compactor.

#### Scenario: reporter passthrough
- **WHEN** `playwright` is invoked with explicit machine-readable or unsupported reporter modes
- **THEN** the phase returns passthrough behavior.

### Requirement: Playwright Runtime Handling
The `playwright` phase SHALL preserve bounded test-result and failure identity information while reducing repetitive run output.

#### Scenario: compact test summary
- **WHEN** supported Playwright output is compacted
- **THEN** output includes bounded pass/fail counts and preserved failed test identities.

#### Scenario: diagnostics and exit parity
- **WHEN** Playwright emits diagnostics or exits non-zero
- **THEN** actionable diagnostics remain visible and native exit-code parity is preserved.

#### Scenario: low-confidence fallback
- **WHEN** Playwright output does not match a recognized safe human-readable test shape
- **THEN** raw buffered output is flushed unchanged.
