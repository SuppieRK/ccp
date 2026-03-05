## Purpose
Define `pip` filter prepare routing, substitution metadata, and JSON compaction behavior.

## Requirements

### Requirement: pip Tool Identity And Aliases
The `pip` filter SHALL identify as `pip` and support common pip executable aliases.

#### Scenario: alias executables
- **WHEN** executable is `pip3`, `pip.exe`, `./pip.exe`, `pip3.exe`, `./pip3.exe`, `pip.cmd`, `./pip.cmd`, `pip3.cmd`, or `./pip3.cmd`
- **THEN** the pip filter contract is used.

### Requirement: pip Subcommand Coverage
The `pip` phase SHALL compact read workflows (`list`, `outdated`) and keep other workflows passthrough.

#### Scenario: list/outdated normalization
- **WHEN** subcommand is `list` or `outdated` without explicit format
- **THEN** args are normalized to JSON-capable shape.
- **AND** dispatch key is `pip|mode=list` or `pip|mode=outdated`.

#### Scenario: explicit format safety
- **WHEN** `--format` is explicitly set to non-JSON
- **THEN** invocation is passthrough ambiguous.

#### Scenario: explicit json safety
- **WHEN** `--format=json` is explicitly provided
- **THEN** invocation is passthrough ambiguous (precision-mode safety).

#### Scenario: compatibility-sensitive flags
- **WHEN** args contain `--editable`/`-e`/`--user`
- **THEN** invocation is passthrough ambiguous.

#### Scenario: install/uninstall/show passthrough
- **WHEN** subcommand is `install`, `uninstall`, or `show`
- **THEN** filter marks passthrough while still providing `uv` preferred substitution hints.

#### Scenario: empty or unsupported subcommand passthrough
- **WHEN** args are empty or subcommand is unsupported by pip compaction flow
- **THEN** invocation is passthrough.

### Requirement: Preferred Substitution Metadata
The pip phase SHALL provide `uv` substitution hints for supported subcommands.

#### Scenario: supported substitution hints
- **WHEN** subcommand is `list`, `outdated`, `install`, `uninstall`, or `show`
- **THEN** prepare result includes `PreferredSubstitution=uv` with corresponding preferred and fallback args.

### Requirement: Structured JSON Compaction
The pip phase SHALL compact parseable JSON arrays and preserve surrounding diagnostic text.

#### Scenario: envelope extraction
- **WHEN** output contains prefix/suffix text around a JSON array payload
- **THEN** compaction parses the array and reattaches preserved prefix/suffix lines.
- **AND** array-boundary detection ignores bracket characters that appear inside quoted JSON strings (including escaped quotes).

#### Scenario: list summary
- **WHEN** `pip|mode=list` payload parses
- **THEN** output is `pip list: <N> packages` plus sorted bounded package lines and optional `... +N more`.

#### Scenario: outdated summary
- **WHEN** `pip|mode=outdated` payload parses
- **THEN** output is `pip outdated: <N> packages` plus sorted bounded `<name> <current> -> <latest>` lines.

#### Scenario: parse-confidence fallback
- **WHEN** JSON parsing or required fields fail, or output is low-confidence
- **THEN** raw buffered output is flushed unchanged.

### Requirement: pip Runtime Exit Handling
The pip phase SHALL collect line/tick/EOF events and flush decisions on exit.

#### Scenario: pre-exit collection
- **WHEN** event type is line, tick, or EOF
- **THEN** event is collected.

#### Scenario: exit empty output
- **WHEN** exit is received and buffered output is empty
- **THEN** no output is emitted.

#### Scenario: empty compact result fallback
- **WHEN** compaction returns empty output
- **THEN** raw buffered output is flushed unchanged.
