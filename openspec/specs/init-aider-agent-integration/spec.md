## ADDED Requirements

### Requirement: Detect Aider Repositories

`ccp init` MUST treat a repository-local `.aider.conf.yml` file as the initial detection heuristic for Aider integration.

#### Scenario: Detect Aider from repository config

- **WHEN** the working repository contains `.aider.conf.yml`
- **AND** the user runs `ccp init` without an explicit `--tools` override
- **THEN** Aider is eligible for auto-detection as a supported integration target

### Requirement: Ensure AGENTS.md Is Loaded by Aider

`ccp init --tools aider` MUST ensure that `.aider.conf.yml` loads `AGENTS.md` through Aider’s persistent `read` configuration.

#### Scenario: Add AGENTS.md to empty or missing config

- **WHEN** the user runs `ccp init --tools aider`
- **AND** `.aider.conf.yml` is missing or does not configure `read`
- **THEN** `ccp` writes config that causes Aider to load `AGENTS.md`

#### Scenario: Preserve existing read entries

- **WHEN** `.aider.conf.yml` already contains other `read` entries
- **AND** the user runs `ccp init --tools aider`
- **THEN** `ccp` preserves the existing entries
- **AND** adds `AGENTS.md` without duplication

#### Scenario: Reapply managed config change deterministically

- **WHEN** `.aider.conf.yml` already includes the CCP-managed Aider config contribution
- **AND** the user runs `ccp init --tools aider` again
- **THEN** the resulting config remains deterministic

### Requirement: Verify Aider Integration

`ccp init` verification MUST confirm that `.aider.conf.yml` still causes Aider to load `AGENTS.md`.

#### Scenario: Verify AGENTS.md read configuration

- **WHEN** `.aider.conf.yml` contains config that loads `AGENTS.md`
- **THEN** verification succeeds for the Aider integration

### Requirement: Remove Only CCP-Managed Aider Config State

`ccp uninstall --tools aider` MUST remove only the CCP-managed Aider config contribution and preserve unrelated Aider configuration.

#### Scenario: Remove AGENTS.md from read configuration

- **WHEN** the user runs `ccp uninstall --tools aider`
- **THEN** `ccp` removes `AGENTS.md` from the managed Aider `read` configuration
- **AND** preserves unrelated `read` entries and unrelated config settings

#### Scenario: Remove config file only when empty

- **WHEN** `.aider.conf.yml` only contains the CCP-managed Aider contribution
- **AND** the user runs `ccp uninstall --tools aider`
- **THEN** `ccp` may remove `.aider.conf.yml`
