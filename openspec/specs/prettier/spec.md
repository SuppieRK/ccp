# prettier Specification

## Purpose
Define direct `prettier` compaction behavior for supported human-readable invocation shapes.

## Requirements
### Requirement: Prettier Planning Safety
The direct `prettier` phase SHALL compact only supported human-readable invocation shapes.

#### Scenario: direct prettier dispatch
- **WHEN** `prettier` is invoked in a supported file-oriented reporting mode
- **THEN** the phase uses the direct `prettier` compactor

#### Scenario: supported direct prettier shapes remain recognized
- **WHEN** a user runs direct `prettier` with supported `--check` or `--write` file-oriented arguments
- **THEN** CCP recognizes the command shape and selects the direct prettier filter path.

#### Scenario: unsupported shape passthrough
- **WHEN** the direct `prettier` invocation shape is unsupported or precision-sensitive
- **THEN** the phase returns passthrough behavior

#### Scenario: unsupported direct prettier shapes still passthrough
- **WHEN** a user runs direct `prettier` with unsupported flags, ambiguous options, or no file paths
- **THEN** CCP keeps the command on passthrough-safe behavior.

### Requirement: Prettier Runtime Handling
The direct `prettier` phase SHALL preserve bounded file-oriented status information while reducing repetitive output.

#### Scenario: compact prettier status output
- **WHEN** supported direct `prettier` output is compacted
- **THEN** output preserves bounded file identity and formatting-status signal in a line-oriented form

#### Scenario: diagnostics and exit parity
- **WHEN** direct `prettier` emits diagnostics or exits non-zero
- **THEN** actionable diagnostics remain visible and native exit-code parity is preserved
