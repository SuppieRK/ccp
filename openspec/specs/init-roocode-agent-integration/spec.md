## ADDED Requirements

### Requirement: Detect RooCode Repositories

`ccp init` MUST treat a repository-local `.roo` directory as the initial detection heuristic for RooCode integration.

#### Scenario: Detect RooCode from repository marker

- **WHEN** the working repository contains a `.roo` directory
- **AND** the user runs `ccp init` without an explicit `--tools` override
- **THEN** RooCode is eligible for auto-detection as a supported integration target

### Requirement: Install CCP-Managed RooCode Rule

`ccp init --tools roocode` MUST install CCP-managed guidance at `.roo/rules/ccp.md`.

#### Scenario: Install managed rule file

- **WHEN** the user runs `ccp init --tools roocode`
- **THEN** `ccp` creates `.roo/rules/ccp.md` if needed
- **AND** writes deterministic CCP-managed rule content to that file
- **AND** does not modify unrelated RooCode files

#### Scenario: Reapply managed rule file

- **WHEN** `.roo/rules/ccp.md` already exists from a previous CCP-managed install
- **AND** the user runs `ccp init --tools roocode` again
- **THEN** `ccp` rewrites only `.roo/rules/ccp.md`
- **AND** the resulting file content remains deterministic

### Requirement: Verify RooCode Integration

`ccp init` verification MUST confirm that the CCP-managed RooCode rule file exists and still contains the expected CCP-managed guidance.

#### Scenario: Verify managed RooCode rule file

- **WHEN** `.roo/rules/ccp.md` exists with intact CCP-managed content
- **THEN** verification succeeds for the RooCode integration

### Requirement: Uninstall Only CCP-Managed RooCode Rule

`ccp uninstall --tools roocode` MUST remove only the CCP-managed rule file and preserve unrelated RooCode files and directories.

#### Scenario: Remove only managed rule file

- **WHEN** the user runs `ccp uninstall --tools roocode`
- **THEN** `ccp` removes `.roo/rules/ccp.md`
- **AND** leaves `.roo`, `.roo/rules`, `.roorules`, and unrelated RooCode files untouched
