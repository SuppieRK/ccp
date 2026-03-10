# init-kilocode-agent-integration Specification

## Purpose
Define the managed Kilocode integration installed and removed by `ccp init` and `ccp uninstall`.

## Requirements
### Requirement: Kilocode Detection vs Install Scope
Kilocode init integration SHALL detect from repository scope and install through a home-scoped Kilo CLI configuration path.

#### Scenario: repository detection with home-scoped kilo cli install
- **WHEN** init resolves tool adapters for `kilocode`
- **THEN** Kilocode detection is based on repository `.kilocode` directory presence
- **AND** installation target resolves through the Kilo home config root under `~/.config/kilocode/`

### Requirement: Kilocode Reuses OpenCode-Family Integration Shape
Kilocode init integration SHALL use an OpenCode-family CLI integration shape while preserving Kilo-specific config placement.

#### Scenario: install managed kilo cli integration
- **WHEN** user runs `ccp init --tools kilocode`
- **THEN** CCP installs the managed integration under `~/.config/kilocode/`
- **AND** it preserves Kilo-specific config placement instead of assuming `~/.config/opencode/` is the correct target
- **AND** OpenCode-family plugin artifacts, if used, are installed under the Kilo-specific config root

### Requirement: Kilocode Idempotent Reapply
Kilocode adapter SHALL be idempotent on repeated runs.

#### Scenario: re-run kilocode init
- **WHEN** `ccp init --tools kilocode` is run twice
- **THEN** the second run does not create duplicate CCP-managed Kilo configuration
- **AND** reports no-op or already-configured status when the managed content is unchanged.
