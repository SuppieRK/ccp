## Purpose
Define lifecycle and integration subcommands for `ccp` outside execution wrapping.
## Requirements
### Requirement: Init Command for Agent Tool Setup
The CLI SHALL provide `ccp init` without lifecycle scope flags to persist tool-specific configuration and proxy preferences (for example `--tools codex,opencode`).

#### Scenario: Managed initialization state
- **WHEN** a user runs `ccp init --tools codex,opencode` from a repository
- **THEN** `ccp` writes its managed `init.json` manifest under the home-scoped config path
- **AND** selected integrations are installed according to each adapter contract.

#### Scenario: Init detects tools from current repository
- **WHEN** a user runs `ccp init` without `--tools`
- **THEN** tool detection is based on the current repository
- **AND** resulting managed state is still written under the home-scoped config path.

### Requirement: Lifecycle Command Dispatch
The CLI SHALL dispatch lifecycle commands (`init`, `gain`, `history`, `upgrade`, `uninstall`) before wrapped execution routing.

#### Scenario: lifecycle command handling precedence
- **WHEN** the first command token is a lifecycle command
- **THEN** the corresponding lifecycle handler is executed
- **AND** wrapped command execution is not invoked.

#### Scenario: uninstall persists init-state cleanup
- **WHEN** a lifecycle `uninstall` command removes one or more configured tools
- **THEN** persisted init configuration is updated to remove those tools
- **AND** the config file is removed when no configured tools remain.
- **AND** init-state cleanup is applied only after tool uninstall actions complete without adapter uninstall errors.

### Requirement: Init Idempotency and Backup
Initialization SHALL be idempotent and SHALL back up existing relevant configuration before modification.

#### Scenario: Re-running init
- **WHEN** `ccp init` is executed repeatedly with the same parameters
- **THEN** resulting configuration remains stable without duplicate entries.

#### Scenario: Existing config preservation
- **WHEN** initialization modifies existing hook or integration config
- **THEN** prior state is backed up before changes are written.

### Requirement: Upgrade command resolves release-backed updates
The upgrade command SHALL resolve binary updates from GitHub Releases and perform in-place self-update without requiring a local source path argument.

#### Scenario: Upgrade latest from releases
- **WHEN** a user runs `ccp upgrade` with no explicit version
- **THEN** `ccp` resolves release metadata via `https://api.github.com/repos/SuppieRK/ccp/releases/latest`, excluding prereleases, and updates the local binary in place.

#### Scenario: Upgrade specific release tag
- **WHEN** a user runs `ccp upgrade --version <tag>`
- **THEN** `ccp` resolves the matching tagged release asset from the canonical repository using the release-by-tag endpoint and updates the local binary in place.

#### Scenario: Upgrade does not support runtime repository override
- **WHEN** a user attempts to provide repository override input for `ccp upgrade`
- **THEN** `ccp upgrade` still resolves releases from the canonical repository contract.

### Requirement: Gain Reporting
The CLI SHALL provide `ccp gain` for summary savings and `ccp history` for execution history, with independent data-selection filters and output-representation formats.

#### Scenario: Gain default rendering
- **WHEN** a user runs `ccp gain` without explicit `--format`
- **THEN** the CLI renders summary output in text format.

#### Scenario: Gain summary output
- **WHEN** a user runs `ccp gain`
- **THEN** the CLI returns summary rows for the selected dataset
- **AND** includes an additional total aggregate across all returned rows.

#### Scenario: Gain text output includes filter header and total row
- **WHEN** a user runs `ccp gain --format text`
- **THEN** the CLI prints active filter metadata (`since`, `tool`, `failed`, `period`) before tabular rows
- **AND** includes one row per grouped tool with compact columns: tool, invocation count, estimated native/proxied tokens, and estimated savings percent
- **AND** prefixes estimated savings percent values with `~` in text output
- **AND** appends a trailing `TOTAL` row when data exists.

#### Scenario: Gain text table is compact and aligned
- **WHEN** `ccp gain --format text` prints summary rows
- **THEN** the output uses stable column ordering and aligned cell widths for readability in terminal output.

#### Scenario: History command output
- **WHEN** a user runs `ccp history` with optional `--since`, `--tool`, or `--failed`
- **THEN** the CLI returns matching execution records ordered by time
- **AND** each record includes command, tool, exit outcome, passthrough status, byte metrics, and estimated token metrics.

#### Scenario: History text output exposes passthrough and ordering
- **WHEN** a user runs `ccp history --format text`
- **THEN** each row includes compact operational columns (timestamp, command, status markers, and estimated savings percent)
- **AND** the text table omits the `tool` column
- **AND** optional numeric detail columns may be reduced in text mode for readability
- **AND** rows are ordered newest-first by default.

#### Scenario: Summary and history share envelope schema
- **WHEN** output is rendered in machine-readable form for `ccp gain` or `ccp history`
- **THEN** both datasets use the same top-level envelope fields (`dataset`, `period`, `filters`, `rows`)
- **AND** `summary` includes an additional `total` aggregate object.

#### Scenario: Output format selection
- **WHEN** a user supplies `--format text|json|csv` on `ccp gain` or `ccp history`
- **THEN** the CLI serializes the selected dataset in the requested format
- **AND** invalid format values fail with an explanatory error.

#### Scenario: Legacy JSON flag alias
- **WHEN** a user supplies legacy `--json` on reporting commands
- **THEN** output format resolves to JSON semantics equivalent to `--format json`.

#### Scenario: CSV includes trailing total row for summary
- **WHEN** `ccp gain` summary data is rendered in CSV
- **THEN** the output includes summary rows for the selected dataset
- **AND** appends a trailing total row covering the full selected dataset.

#### Scenario: Period selection is independent of format
- **WHEN** a user supplies `--period day|week|month` with any valid gain format (`text|json|csv`)
- **THEN** the CLI applies period-based data aggregation for selection
- **AND** renders the selected dataset using the requested format.

#### Scenario: Shared filters apply to selected dataset
- **WHEN** a user supplies any combination of `--since`, `--tool`, and `--failed` on reporting commands
- **THEN** those filters are applied to the selected dataset before rendering
- **AND** filtering semantics are consistent across supported `--format` values.

#### Scenario: Failed filter semantics
- **WHEN** a user enables `--failed`
- **THEN** only records with `exit_code != 0` are included in the selected dataset.

#### Scenario: Estimated token labeling
- **WHEN** gain output includes token counts
- **THEN** the output labels those values as estimated using a 4-bytes-per-token heuristic.

#### Scenario: Empty text dataset behavior
- **WHEN** `ccp gain --format text` or `ccp history --format text` resolves to zero rows
- **THEN** the CLI prints a no-results message
- **AND** still prints active filter metadata.

### Requirement: Version Interface
The CLI SHALL expose `--version`.

#### Scenario: Version output
- **WHEN** a user runs `ccp --version`
- **THEN** the CLI prints the current installed version and exits successfully.

### Requirement: Help Interface
The CLI SHALL expose `--help` and `-h`.

#### Scenario: Help output
- **WHEN** a user runs `ccp --help` or `ccp -h`
- **THEN** the CLI prints usage text including supported top-level flags and command forwarding shape
- **AND** exits successfully without executing lifecycle or wrapped commands.

#### Scenario: Help bypasses execution-scope validation
- **WHEN** `--help` is present together with execution-scope flags
- **THEN** argument validation errors specific to wrapped execution scope are not emitted
- **AND** usage text is printed successfully.

### Requirement: Positive Integer Option Parsing
Option helpers that read numeric flag values SHALL only accept positive integers and SHALL fall back to the provided default when the flag value is missing, non-numeric, or non-positive.

#### Scenario: Positive integer value from spaced option form
- **WHEN** a wrapped command includes a numeric option as `<flag> <value>` and `<value>` is a positive integer
- **THEN** the parsed value is returned.

#### Scenario: Positive integer value from inline option form
- **WHEN** a wrapped command includes a numeric option as `<flag>=<value>` and `<value>` is a positive integer
- **THEN** the parsed value is returned.

#### Scenario: Invalid numeric option uses fallback
- **WHEN** a wrapped command includes a numeric option with missing, non-numeric, zero, or negative value
- **THEN** the helper returns the provided fallback value.
