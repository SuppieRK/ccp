# init-roocode-agent-integration Specification

## Purpose
Define how `ccp init` detects, installs, verifies, and uninstalls the RooCode integration.

## Requirements
### Requirement: Detect RooCode Repositories

`ccp init` MUST treat a repository-local `.roo` directory as the initial detection heuristic for RooCode integration.

#### Scenario: Detect RooCode from repository marker

- **WHEN** the working repository contains a `.roo` directory
- **AND** the user runs `ccp init` without an explicit `--tools` override
- **THEN** RooCode is eligible for auto-detection as a supported integration target

### Requirement: Install CCP-Managed RooCode Rule

`ccp init --tools roocode` MUST install CCP-managed guidance in the canonical home-scoped `.roo` rules directory.

#### Scenario: install managed home roocode rule

- **WHEN** the user runs `ccp init --tools roocode`
- **THEN** `ccp` creates the canonical managed rule under the home-scoped `.roo` rules directory if needed
- **AND** writes deterministic CCP-managed rule content to that file
- **AND** does not modify unrelated RooCode files

#### Scenario: reapply managed home roocode rule

- **WHEN** the canonical home-scoped RooCode managed rule already exists from a previous CCP-managed install
- **AND** the user runs `ccp init --tools roocode` again
- **THEN** `ccp` rewrites only the managed home-scoped rule
- **AND** the resulting file content remains deterministic

### Requirement: Verify RooCode Integration

`ccp init` verification MUST confirm that the CCP-managed home-scoped RooCode rule exists and still contains the expected guidance.

#### Scenario: verify managed home roocode rule

- **WHEN** the home-scoped RooCode managed rule exists with intact CCP-managed content
- **THEN** verification succeeds for the RooCode integration

### Requirement: Uninstall Only CCP-Managed RooCode Rule

`ccp uninstall --tools roocode` MUST remove only the CCP-managed home-scoped RooCode rule and preserve unrelated RooCode files and directories.

#### Scenario: remove only managed home roocode rule

- **WHEN** the user runs `ccp uninstall --tools roocode`
- **THEN** `ccp` removes only the managed home-scoped RooCode rule
- **AND** leaves unrelated RooCode files and directories untouched
