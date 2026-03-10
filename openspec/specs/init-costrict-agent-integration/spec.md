# init-costrict-agent-integration Specification

## Purpose
Define how `ccp init` accepts `costrict` as an alias for the existing RooCode integration.

## Requirements
### Requirement: Normalize `costrict` to RooCode

`ccp init` and `ccp uninstall` MUST treat `costrict` as an alias of the canonical RooCode integration target.

#### Scenario: init alias resolves to canonical roocode target
- **WHEN** the user runs `ccp init --tools costrict`
- **THEN** CCP accepts `costrict` as a valid tool selection
- **AND** normalizes it to the RooCode integration path
- **AND** installs CCP-managed guidance into the canonical RooCode target under `~/.roo/rules/`

#### Scenario: uninstall alias resolves to canonical roocode target
- **WHEN** the user runs `ccp uninstall --tools costrict`
- **THEN** CCP accepts `costrict` as a valid tool selection
- **AND** removes the same CCP-managed RooCode artifacts it would remove for `roocode`

### Requirement: Preserve Singular `.roo` Auto-Detection

Auto-detection for `.roo` projects MUST remain singular.

#### Scenario: detect `.roo` project without duplicate alias tool
- **WHEN** the current repository contains a `.roo` directory
- **THEN** CCP does not auto-detect both `roocode` and `costrict`
- **AND** only one tool identity is selected for lifecycle execution

### Requirement: Reuse Existing RooCode Managed Path

The `costrict` alias MUST reuse RooCode's canonical managed target at the home-scoped `.roo` rules directory.

#### Scenario: rerun remains idempotent through alias
- **GIVEN** the canonical RooCode managed guidance already exists in the home-scoped `.roo` rules directory
- **WHEN** the user reruns `ccp init --tools costrict`
- **THEN** CCP does not duplicate the managed content
- **AND** the resulting configuration remains semantically unchanged
