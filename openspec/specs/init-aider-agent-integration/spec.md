# init-aider-agent-integration Specification

## Purpose
Define how `ccp init` detects, installs, verifies, and uninstalls the Aider integration.

## Requirements
### Requirement: Detect Aider Repositories

`ccp init` MUST treat a repository-local `.aider.conf.yml` file as the initial detection heuristic for Aider integration.

#### Scenario: Detect Aider from repository config

- **WHEN** the working repository contains `.aider.conf.yml`
- **AND** the user runs `ccp init` without an explicit `--tools` override
- **THEN** Aider is eligible for auto-detection as a supported integration target

### Requirement: Ensure AGENTS.md Is Loaded by Aider

`ccp init --tools aider` MUST ensure that Aider loads CCP-managed guidance from a home-scoped markdown file through the user's home-scoped config.

#### Scenario: install home-scoped aider guidance

- **WHEN** the user runs `ccp init --tools aider`
- **THEN** `ccp` resolves `~/.aider.conf.yml` as the canonical Aider config target
- **AND** `ccp` ensures that config loads a home-scoped guidance file such as `~/.aider.rules.md`

#### Scenario: preserve existing home config entries

- **WHEN** `~/.aider.conf.yml` already contains other `read` entries
- **AND** the user runs `ccp init --tools aider`
- **THEN** `ccp` preserves the existing entries
- **AND** adds the CCP-managed home guidance file without duplication

#### Scenario: Reapply managed config change deterministically

- **WHEN** `~/.aider.conf.yml` already includes the CCP-managed Aider config contribution
- **AND** the user runs `ccp init --tools aider` again
- **THEN** the resulting config remains deterministic

### Requirement: Verify Aider Integration

`ccp init` verification MUST confirm that the home-scoped Aider config still loads the CCP-managed home guidance file.

#### Scenario: verify home aider read configuration

- **WHEN** `~/.aider.conf.yml` contains config that loads the CCP-managed home guidance file
- **THEN** verification succeeds for the Aider integration

### Requirement: Remove Only CCP-Managed Aider Config State

`ccp uninstall --tools aider` MUST remove only the CCP-managed home-scoped Aider config contribution and preserve unrelated Aider configuration.

#### Scenario: remove managed home guidance reference

- **WHEN** the user runs `ccp uninstall --tools aider`
- **THEN** `ccp` removes the CCP-managed home guidance file reference from `~/.aider.conf.yml`
- **AND** preserves unrelated `read` entries and unrelated config settings

#### Scenario: Remove config file only when empty

- **WHEN** `~/.aider.conf.yml` only contains the CCP-managed Aider contribution
- **AND** the user runs `ccp uninstall --tools aider`
- **THEN** `ccp` may remove `~/.aider.conf.yml`
