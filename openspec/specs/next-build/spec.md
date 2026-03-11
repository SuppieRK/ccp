## Purpose
Define direct `next build` planning safety and runtime build-summary compaction behavior.

## Requirements

### Requirement: Next Build Planning Safety
The `next build` phase SHALL compact only supported direct build invocation shapes.

#### Scenario: direct next build dispatch
- **WHEN** `next build` is invoked in a supported default human-readable mode
- **THEN** the phase uses the Next.js build compactor.

#### Scenario: unsupported build passthrough
- **WHEN** the invocation shape is unsupported or precision-sensitive
- **THEN** the phase returns passthrough behavior.

### Requirement: Next Build Runtime Handling
The `next build` phase SHALL preserve build outcome and actionable diagnostics while reducing repetitive progress output.

#### Scenario: compact build summary
- **WHEN** supported `next build` output is compacted
- **THEN** output includes bounded build completion or failure summary information.

#### Scenario: compact route and bundle table
- **WHEN** `next build` emits recognizable route or bundle summary tables
- **THEN** the phase preserves bounded route and bundle summary information instead of raw table verbosity.

#### Scenario: cached build summary
- **WHEN** `next build` reports a recognizable cached or already-built success shape
- **THEN** the phase preserves the successful cached-build outcome in compact form.

#### Scenario: diagnostics and exit parity
- **WHEN** `next build` emits diagnostics or exits non-zero
- **THEN** actionable diagnostics remain visible and native exit-code parity is preserved.

#### Scenario: low-confidence fallback
- **WHEN** `next build` output does not match a recognized safe build shape
- **THEN** the phase flushes raw buffered output unchanged.
