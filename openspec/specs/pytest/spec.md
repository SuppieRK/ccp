## Purpose
Define `pytest` filter identity, prepare normalization, and deterministic runtime compaction behavior.

## Requirements

### Requirement: Pytest Tool Identity And Aliases
The `pytest` filter SHALL identify as `pytest` and support platform aliases.

#### Scenario: alias executables
- **WHEN** executable is `pytest.exe`, `./pytest.exe`, `pytest.cmd`, or `./pytest.cmd`
- **THEN** the pytest filter contract is used.

### Requirement: Pytest Prepare Default Normalization
The filter SHALL append compact defaults only when explicit troubleshooting intent is absent.

#### Scenario: compact-default injection
- **WHEN** args do not include traceback controls and do not include verbose controls
- **THEN** `--tb=short` is appended.
- **AND** `--no-header` is appended when neither `--header` nor `--no-header` is present.

#### Scenario: explicit troubleshooting intent preserved
- **WHEN** args include traceback flag (`--tb`/`--tb=...`) or verbose flag (`--verbose` or `-v+`)
- **THEN** args are preserved and compact defaults are not appended.

### Requirement: Pytest Runtime Event Handling
The filter SHALL keep stderr immediate and compact stdout only on process exit.

#### Scenario: stderr immediate passthrough
- **WHEN** stderr line events arrive
- **THEN** lines are emitted immediately unchanged.

#### Scenario: stdout pre-exit collection
- **WHEN** stdout line or tick events arrive
- **THEN** output is collected for exit-time compaction.

#### Scenario: stdout EOF handling
- **WHEN** stdout EOF arrives before process exit
- **THEN** no output is emitted at EOF.

#### Scenario: stdout exit compaction and empty-buffer parity
- **WHEN** stdout exit arrives with non-empty buffered output
- **THEN** compacted output is flushed for recognized pytest shapes.
- **AND** buffered raw stdout is flushed unchanged when parse confidence is low.
- **WHEN** stdout exit arrives with empty buffered output
- **THEN** no output is emitted.

### Requirement: Pytest Compaction Outcomes
The filter SHALL emit deterministic summaries for recognized pytest output.

#### Scenario: deterministic outcome selection
- **WHEN** parse is recognized
- **THEN** compaction selects exactly one outcome renderer: no-tests, pass-only/complete, or failure summary.

#### Scenario: no tests collected
- **WHEN** parse detects no-tests outcome
- **THEN** output is `pytest: no tests collected`.

#### Scenario: pass summary
- **WHEN** parse recognizes pass-only outcome
- **THEN** output is `pytest: <passed> passed[, <skipped> skipped]`.

#### Scenario: complete summary without pass/fail/error counts
- **WHEN** parse is recognized but has zero passed, failed, and errors counts
- **THEN** output is `pytest: complete`.

#### Scenario: failure summary
- **WHEN** parse recognizes failed/error outcome
- **THEN** output includes overall totals and `failure details:`.
- **AND** at most first 3 detailed failure blocks are emitted.
- **AND** `failed tests:` list is emitted from short-summary entries.

#### Scenario: failure context and capture bounds
- **WHEN** failure block source/capture sections are present
- **THEN** context keeps up to 3 lines before and 3 lines after failing source marker.
- **AND** captured stdout/stderr retention is bounded to 12 lines per failure block.
