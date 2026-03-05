## Purpose
Define `find` filter execution compatibility, argument parsing, and deterministic compaction behavior.

## Requirements

### Requirement: Find Execution And Compatibility
The `find` phase SHALL preserve native execution semantics while enabling safe substitution hints.

#### Scenario: native command execution
- **WHEN** a `find` command is planned
- **THEN** normalized args preserve requested find shape.

#### Scenario: optional `fd` substitution hint
- **WHEN** parsed args are substitution-safe (no unsupported expressions like `-exec`, `-prune`, `-delete`, `-o`/`-or`)
- **THEN** planner emits preferred substitution `fd` with fallback args.

### Requirement: Find Argument Parsing
The `find` phase SHALL parse custom compaction controls and common expression forms.

#### Scenario: custom controls
- **WHEN** args include `--all`, `--heartbeat`, or `--max-results[/_]`
- **THEN** those controls are consumed into dispatch config.

#### Scenario: expression extraction
- **WHEN** args include `-name/-iname/-path/-ipath` and `-type`
- **THEN** dispatch captures pattern and file-type (`f`/`d`) for compaction filtering.

#### Scenario: default root
- **WHEN** no explicit path root is provided
- **THEN** root defaults to `.`.

### Requirement: Find Compaction Filtering
The filter SHALL compact emitted paths by root-relative grouping with optional hidden-path filtering.

#### Scenario: hidden-path filtering
- **WHEN** dispatch sets `hidden=0`
- **THEN** entries with hidden path segments are excluded.

#### Scenario: file-vs-directory mode
- **WHEN** dispatch `type=d`
- **THEN** directory list rendering is used.
- **WHEN** dispatch type is file mode
- **THEN** grouped file rendering (`dir/` + indented names) is used.

#### Scenario: bounded rendering
- **WHEN** result count exceeds dispatch max
- **THEN** output is truncated with `+N more`.

#### Scenario: heartbeat tick
- **WHEN** heartbeat is enabled and tick events occur with buffered content
- **THEN** interim progress line `[find] scanned <N> paths...` may be emitted.

#### Scenario: low-confidence fallback
- **WHEN** output includes low-confidence markers (for example NUL bytes)
- **THEN** compaction falls back to raw passthrough.
