# installer-distribution Specification

## Purpose
Define deterministic installer behavior for release resolution, platform asset selection, and install-path setup.

## Requirements
### Requirement: Installer uses canonical repository only
The installer SHALL resolve release metadata and download assets from a fixed canonical GitHub repository embedded in installer logic.

#### Scenario: Installer resolves latest version from canonical repository
- **WHEN** `scripts/install.sh` runs with default version behavior
- **THEN** it queries `https://api.github.com/repos/SuppieRK/ccp/releases/latest`
- **AND** it extracts `tag_name` from that response before selecting download assets.

#### Scenario: Installer resolves explicit version from canonical repository
- **WHEN** `scripts/install.sh` runs with `VERSION=<tag>`
- **THEN** it downloads `ccp_<tag>_<os>_<arch>.zip` from `SuppieRK/ccp` release assets.

### Requirement: Installer parsing and dependency model is shell-native
The installer SHALL parse release metadata using shell-native tooling and SHALL NOT require `jq`.

#### Scenario: Latest tag parsing uses shell tooling
- **WHEN** latest release metadata is fetched
- **THEN** installer parses `tag_name` using shell-native text tooling
- **AND** `jq` is not required for installer execution.

#### Scenario: Required command dependencies are validated
- **WHEN** installer starts
- **THEN** it checks for required commands used by the script (`uname`, `curl`, `unzip`)
- **AND** it fails fast if a required command is missing.

### Requirement: Installer does not expose repository or install-dir override
The installer SHALL NOT support runtime repository override or install-directory override variables.

#### Scenario: Repository override is unsupported
- **WHEN** a user attempts to provide repository override inputs
- **THEN** installer remains bound to canonical repository `SuppieRK/ccp`.

#### Scenario: Install directory override is unsupported
- **WHEN** a user attempts to provide install-dir override inputs
- **THEN** installer continues deterministic directory selection logic.

### Requirement: Installer uses deterministic install location selection
Installer SHALL select destination directory in a fixed order.

#### Scenario: Installer selects first writable target in deterministic order
- **WHEN** installer determines destination for binary placement
- **THEN** it selects the first writable/creatable path in order:
  - `/usr/local/bin`
  - `$HOME/.local/bin`
  - `$(pwd)/bin`.

#### Scenario: Installer fails when no target directory can be selected
- **WHEN** none of `/usr/local/bin`, `$HOME/.local/bin`, or local `./bin` is writable/creatable
- **THEN** installer exits with an install-directory selection failure.

### Requirement: Installer ensures PATH discoverability for non-system installs
The installer SHALL attempt idempotent PATH configuration when install target is not already on PATH and not `/usr/local/bin`.

#### Scenario: Bash and Zsh profile updates are idempotent
- **WHEN** shell is Bash or Zsh and target directory is not on PATH
- **THEN** installer appends one PATH export block to the selected profile file only if not already present.

#### Scenario: Fish shell uses fish_add_path
- **WHEN** shell is Fish and target directory is not on PATH
- **THEN** installer uses `fish_add_path` for PATH setup.

#### Scenario: Unknown shell falls back to .profile
- **WHEN** shell is not Bash/Zsh/Fish and target directory is not on PATH
- **THEN** installer appends PATH export block to `$HOME/.profile`.

### Requirement: Release workflow smoke-tests installer distribution
Release CI SHALL validate installer behavior on Unix and binary availability on Windows.

#### Scenario: Unix smoke-test validates installed binary path options
- **WHEN** `release-distribution.yml` smoke-install job runs on Unix
- **THEN** it runs `scripts/install.sh` with release `VERSION`
- **AND** validates executable availability in `/usr/local/bin/ccp`, `$HOME/.local/bin/ccp`, or `$PWD/bin/ccp`.

#### Scenario: Windows smoke-test validates release archive execution
- **WHEN** `release-distribution.yml` smoke-install job runs on Windows
- **THEN** it downloads `ccp_<version>_windows_amd64.zip` from the release
- **AND** extracts and executes `ccp.exe --version`.
