# init-kiro-agent-integration Specification

## Purpose
Define how `ccp init` detects, installs, verifies, and uninstalls the Kiro integration.

## Requirements
### Requirement: Detect Kiro Repositories

`ccp init` MUST treat a repository-local `.kiro` directory as the initial detection heuristic for Kiro integration.

#### Scenario: Detect Kiro from repository marker

- **WHEN** the working repository contains a `.kiro` directory
- **AND** the user runs `ccp init` without an explicit `--tools` override
- **THEN** Kiro is eligible for auto-detection as a supported integration target

### Requirement: Install CCP-Managed Kiro Steering

`ccp init --tools kiro` MUST install CCP through Kiro's home-scoped global steering surface rather than through `.kiro/steering/ccp.md`.

#### Scenario: install managed home-scoped steering

- **WHEN** the user runs `ccp init --tools kiro`
- **THEN** `ccp` writes CCP-managed guidance into `~/.kiro/steering/AGENTS.md`
- **AND** repository `.kiro` markers remain detection inputs rather than the install target

### Requirement: Verify Kiro Integration

`ccp init` verification MUST confirm that the managed Kiro global steering file still exists and contains the expected CCP behavior.

#### Scenario: verify managed kiro steering

- **WHEN** the managed Kiro global steering file exists with intact CCP behavior
- **THEN** verification succeeds for the Kiro integration

### Requirement: Uninstall Only CCP-Managed Kiro Steering

`ccp uninstall --tools kiro` MUST remove only the CCP-managed Kiro global steering contribution and preserve unrelated Kiro configuration.

#### Scenario: remove only managed steering contribution

- **WHEN** the user runs `ccp uninstall --tools kiro`
- **THEN** `ccp` removes only the managed Kiro global steering contribution
- **AND** preserves unrelated Kiro steering files and configuration
