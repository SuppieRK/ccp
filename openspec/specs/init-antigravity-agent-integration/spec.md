# init-antigravity-agent-integration Specification

## Purpose
Define the managed Antigravity integration installed and removed by `ccp init` and `ccp uninstall`.

## Requirements
### Requirement: Antigravity Detection vs Install Scope
Antigravity init integration SHALL detect from repository scope and install through the Gemini-compatible home-scoped target.

#### Scenario: repository detection with gemini home target
- **WHEN** init resolves tool adapters for `antigravity`
- **THEN** Antigravity detection is based on repository `.agent` directory presence
- **AND** installation target resolves through the Gemini-compatible home path `~/.gemini/GEMINI.md`

### Requirement: Antigravity Reuses Gemini-Family Integration Path
Antigravity integration SHALL reuse the Gemini-family canonical install target and managed guidance behavior.

#### Scenario: antigravity resolves through gemini-family path
- **WHEN** the user runs `ccp init --tools antigravity`
- **THEN** CCP installs the managed guidance at the Gemini-compatible home target
- **AND** it does not create a second long-term Antigravity-specific managed install target

### Requirement: Antigravity Idempotent Reapply
Antigravity adapter SHALL be idempotent on repeated runs.

#### Scenario: re-run antigravity init
- **WHEN** `ccp init --tools antigravity` is run twice
- **THEN** the second run does not create duplicate CCP-managed Gemini-family guidance
- **AND** reports no-op or already-configured status when the managed guidance content is unchanged.
