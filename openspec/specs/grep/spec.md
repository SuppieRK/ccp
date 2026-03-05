## Purpose
Define planner substitution, argument normalization, and runtime compaction behavior for `grep`.

## Requirements

### Requirement: Grep Tool Identity And Substitution
The `grep` filter SHALL identify as `grep`, support `rg` alias dispatch, and prefer `rg` execution when available.

#### Scenario: Preferred substitution
- **WHEN** a `grep` command is planned and `rg` is available
- **THEN** planner executes `rg`
- **AND** tool identity remains `grep` for filter dispatch.

#### Scenario: Preferred substitution fallback
- **WHEN** `rg` is unavailable
- **THEN** planner executes native `grep` with compatibility fallback args.

### Requirement: Grep Argument Normalization
Planner normalization SHALL preserve grep intent while producing deterministic parse shape.

#### Scenario: Deterministic `rg` output shape
- **WHEN** normalized args are built for preferred `rg`
- **THEN** args include `--no-heading`, `--with-filename`, `--line-number`, and `--color=never`.

#### Scenario: BRE alternation translation
- **WHEN** pattern includes `\\|`
- **THEN** pattern is translated to `|` for execution.

#### Scenario: Recursive handling
- **WHEN** input contains `-r` or `--recursive`
- **THEN** those flags are dropped for `rg` normalization
- **AND** preserved for native grep fallback normalization.

#### Scenario: Missing path guardrail
- **WHEN** pattern is provided without an explicit search path
- **THEN** `.` is appended as default path.

### Requirement: Unsafe Regex Translation Safety
Unsafe BRE translation candidates SHALL be marked ambiguous.

#### Scenario: Ambiguous regex in permissive mode
- **WHEN** pattern uses unsupported BRE constructs without ERE/PERL flags
- **THEN** plan is marked ambiguous and forced to neutral passthrough.

#### Scenario: Ambiguous regex in strict mode
- **WHEN** strict mode is enabled for the same ambiguous plan
- **THEN** planning is rejected with explicit ambiguity diagnostics.

### Requirement: Grep Runtime Handling
Runtime processing SHALL preserve diagnostics and compact stdout only at exit.

#### Scenario: stderr diagnostics
- **WHEN** stderr lines are emitted
- **THEN** they are forwarded immediately
- **AND** `rg:`-prefixed diagnostics are normalized to concise `grep:`-prefixed forms when equivalent.

#### Scenario: stdout collection
- **WHEN** line/tick/EOF events arrive on stdout
- **THEN** output is collected.

#### Scenario: low-confidence parse fallback
- **WHEN** compact parsing fails for buffered stdout
- **THEN** raw buffered stdout is flushed unchanged.

### Requirement: Grep Compact Output Rendering
Compacted grep output SHALL group and bound matches deterministically.

#### Scenario: Grouped rendering
- **WHEN** matches are parsed successfully
- **THEN** output is grouped by file and includes line-numbered entries with per-file match counts.

#### Scenario: Context-only truncation
- **WHEN** `context_only` is enabled in dispatch metadata
- **THEN** rendered match text is truncated to bounded context snippets.

#### Scenario: Max-results cap
- **WHEN** parsed matches exceed configured `max`
- **THEN** rendering is capped and appends `... +<n> more matches`.

### Requirement: No-Match Semantics
No-match behavior SHALL follow exit/strict metadata semantics without synthetic errors.

#### Scenario: Strict or non-zero no-match
- **WHEN** buffered stdout is empty and exit is non-zero
- **OR** dispatch includes `strict_no_match=1`
- **THEN** no stdout output is emitted.

#### Scenario: Non-strict zero-exit no-match
- **WHEN** buffered stdout is empty, exit is zero, and strict no-match is disabled
- **THEN** output is `0 matches`.
