# init-crush-agent-integration Specification

## Purpose
Define how `ccp init` installs and manages CCP guidance for Crush CLI projects.

## Requirements
### Requirement: Detect Crush Projects via `.crush`

`ccp init` MUST treat `.crush` as the initial repository-scoped detection heuristic for Crush projects.

#### Scenario: detect Crush project from known directory
- **WHEN** the current repository contains a `.crush` directory
- **THEN** `ccp init` includes `crush` in the detected tool candidates when `--tools` is omitted

### Requirement: Install CCP Guidance Into Repo-Root `AGENTS.md`

The Crush integration MUST install CCP-managed guidance through the home-scoped Crush configuration and user context path rather than through repo-root `AGENTS.md`.

#### Scenario: install home-scoped crush context path
- **WHEN** the user runs `ccp init --tools crush`
- **THEN** CCP resolves `~/.config/crush/crush.json` as the canonical Crush config target
- **AND** CCP ensures that `options.context_paths` includes `~/.config/crush/CRUSH.md`
- **AND** CCP writes or updates the managed home-scoped `CRUSH.md` guidance file

### Requirement: Reapply Crush Guidance Idempotently

Crush installs MUST remain idempotent across repeated `ccp init` runs when using the home-scoped config and context file.

#### Scenario: rerun does not duplicate managed home context wiring
- **GIVEN** `~/.config/crush/crush.json` already references the CCP-managed home `CRUSH.md`
- **WHEN** the user reruns `ccp init --tools crush`
- **THEN** CCP does not duplicate the managed context-path entry
- **AND** the resulting config and home guidance remain semantically unchanged

### Requirement: Remove Only CCP-Managed Crush Content

`ccp uninstall --tools crush` MUST remove only the CCP-managed Crush home context contribution.

#### Scenario: uninstall preserves unrelated crush config
- **GIVEN** `~/.config/crush/crush.json` contains unrelated settings and the CCP-managed context-path entry
- **WHEN** the user runs `ccp uninstall --tools crush`
- **THEN** CCP removes only the managed context-path entry and managed home guidance
- **AND** preserves unrelated Crush configuration
