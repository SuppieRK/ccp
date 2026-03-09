# init-costrict-agent-integration Specification

## Purpose
Define how `ccp init` accepts `costrict` as an alias for the existing RooCode integration.

## Requirements
### Requirement: Normalize `costrict` to RooCode

`ccp init` and `ccp uninstall` MUST treat `costrict` as an alias of the existing RooCode integration.

#### Scenario: init alias resolves to RooCode integration
- **WHEN** the user runs `ccp init --tools costrict`
- **THEN** CCP accepts `costrict` as a valid tool selection
- **AND** normalizes it to the RooCode integration path
- **AND** installs CCP-managed guidance into `.roo/rules/ccp.md`

#### Scenario: uninstall alias resolves to RooCode integration
- **WHEN** the user runs `ccp uninstall --tools costrict`
- **THEN** CCP accepts `costrict` as a valid tool selection
- **AND** removes the same CCP-managed RooCode rule file content it would remove for `roocode`

### Requirement: Preserve Singular `.roo` Auto-Detection

Auto-detection for `.roo` projects MUST remain singular.

#### Scenario: detect `.roo` project without duplicate alias tool
- **WHEN** the current repository contains a `.roo` directory
- **THEN** CCP does not auto-detect both `roocode` and `costrict`
- **AND** only one tool identity is selected for lifecycle execution

### Requirement: Reuse Existing RooCode Managed Path

The `costrict` alias MUST reuse RooCode's existing managed target at `.roo/rules/ccp.md`.

#### Scenario: rerun remains idempotent through alias
- **GIVEN** `.roo/rules/ccp.md` already contains the CCP-managed RooCode guidance
- **WHEN** the user reruns `ccp init --tools costrict`
- **THEN** CCP does not duplicate the managed content
- **AND** the resulting file remains semantically unchanged
