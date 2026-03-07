## ADDED Requirements

### Requirement: Detect Kiro Repositories

`ccp init` MUST treat a repository-local `.kiro` directory as the initial detection heuristic for Kiro integration.

#### Scenario: Detect Kiro from repository marker

- **WHEN** the working repository contains a `.kiro` directory
- **AND** the user runs `ccp init` without an explicit `--tools` override
- **THEN** Kiro is eligible for auto-detection as a supported integration target

### Requirement: Install CCP-Managed Kiro Steering

`ccp init --tools kiro` MUST install CCP-managed guidance at `.kiro/steering/ccp.md`.

#### Scenario: Install managed steering file

- **WHEN** the user runs `ccp init --tools kiro`
- **THEN** `ccp` creates `.kiro/steering/ccp.md` if needed
- **AND** writes deterministic CCP-managed steering content to that file
- **AND** does not modify unrelated Kiro files

#### Scenario: Reapply managed steering file

- **WHEN** `.kiro/steering/ccp.md` already exists from a previous CCP-managed install
- **AND** the user runs `ccp init --tools kiro` again
- **THEN** `ccp` rewrites only `.kiro/steering/ccp.md`
- **AND** the resulting file content remains deterministic

### Requirement: Verify Kiro Integration

`ccp init` verification MUST confirm that the CCP-managed Kiro steering file exists and still contains the expected CCP-managed guidance.

#### Scenario: Verify managed Kiro steering file

- **WHEN** `.kiro/steering/ccp.md` exists with intact CCP-managed content
- **THEN** verification succeeds for the Kiro integration

### Requirement: Uninstall Only CCP-Managed Kiro Steering

`ccp uninstall --tools kiro` MUST remove only the CCP-managed steering file and preserve unrelated Kiro files and directories.

#### Scenario: Remove only managed steering file

- **WHEN** the user runs `ccp uninstall --tools kiro`
- **THEN** `ccp` removes `.kiro/steering/ccp.md`
- **AND** leaves `.kiro`, `.kiro/steering`, and unrelated Kiro files untouched
