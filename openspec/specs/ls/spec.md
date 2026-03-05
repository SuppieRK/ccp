## Purpose
Define `ls` command semantic preparation and compaction behavior implemented by the built-in `ls` filter.

## Requirements

### Requirement: ls Command Preparation
The `ls` filter SHALL normalize arguments only when long-listing intent is explicit.

#### Scenario: long-listing normalization
- **WHEN** args include short long-listing flag `-l` (including grouped forms such as `-lh`/`-lR`)
- **THEN** prepare normalizes to include `-la`
- **AND** duplicate `l`/`a`/`h` and `--all` are removed from forwarded flags
- **AND** remaining extra short/long flags are preserved
- **AND** if no path args remain, `.` is appended.

#### Scenario: non-long passthrough
- **WHEN** args do not include short `-l`
- **THEN** prepare forces passthrough and preserves original args.

### Requirement: ls Output Compaction
The `ls` filter SHALL compact stdout on EOF while preserving stderr diagnostics unchanged.

#### Scenario: Parsed listing transformation
- **WHEN** EOF is received with parseable long-listing output
- **THEN** non-entry header/noise lines (including localized `total`-style headers), empty lines, `.` and `..` entries are omitted
- **AND** each directory entry is rendered as `<name>/`
- **AND** each regular-file or symlink entry is rendered as `<name>  <size>`
- **AND** `<size>` uses normalized units: `B`, `K` (1 decimal), `M` (1 decimal).
- **AND** renderings MAY be packed onto fewer lines for compression, as long as each rendered entry still preserves the same entry-level semantics.

#### Scenario: Short-listing enrichment with fallback
- **WHEN** EOF is received with short-listing style output that lacks type/size metadata
- **THEN** filter MAY enrich entries from filesystem state using dispatch targets
- **AND** if enrichment is not possible, filter emits raw short-listing text without fabricating metadata.

#### Scenario: stderr visibility
- **WHEN** stderr line events are received
- **THEN** lines are emitted immediately unchanged.

#### Scenario: No curated suppression
- **WHEN** `ls` output includes generated, ignored, or intermediate build-artifact paths
- **THEN** those entries remain eligible for compact output
- **AND** the phase does not apply hardcoded noise-directory suppression.

#### Scenario: Summary rendering
- **WHEN** compact output is rendered
- **THEN** output SHOULD end with a plain-text summary line of file and directory counts
- **AND** extension counts MAY be included in descending frequency order
- **AND** at most the top five extensions are rendered, with a `+N more` suffix when applicable.

#### Scenario: Tiny output summary elision
- **WHEN** long-listing compact output contains only a small number of rendered entries
- **THEN** the summary line MAY be omitted to reduce output size
- **AND** entry-level semantics remain unchanged (`<name>/` for directories and `<name>  <size>` for regular-file/symlink entries).

#### Scenario: Plain-text only rendering
- **WHEN** compact output is rendered
- **THEN** output contains plain text only
- **AND** output SHALL NOT include emoji or decorative glyphs.

#### Scenario: Empty compact output
- **WHEN** no directory or file entries remain after filtering
- **THEN** output is `(empty)` followed by a newline.
