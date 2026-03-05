## Purpose
Define `cargo` parent filter routing scope and runtime safety contracts.

## Requirements

### Requirement: Cargo Command Scope
The `cargo` phase SHALL route supported subcommands to dedicated compactors and passthrough unsupported or ambiguous shapes.

#### Scenario: recognized cargo subcommands
- **WHEN** command is `cargo test`, `cargo build`, `cargo check`, or `cargo clippy`
- **THEN** the corresponding specialized compact path is used.

#### Scenario: unsupported cargo passthrough
- **WHEN** cargo subcommand is outside specialized support
- **THEN** execution is forwarded directly and output is not semantically compacted.

#### Scenario: cargo run passthrough
- **WHEN** command is `cargo run ...`
- **THEN** invocation is treated as ambiguous passthrough.
- **AND** application runtime output is not semantically compacted in this phase.

#### Scenario: explicit structured output passthrough
- **WHEN** cargo is invoked with explicit structured output flags (for example `--message-format=json`)
- **THEN** invocation is treated as ambiguous passthrough and cargo semantic compaction is bypassed.

#### Scenario: leading cargo global flags
- **WHEN** cargo global flags/toolchain selectors appear before the subcommand (for example `+stable`, `--config`, `--color`, `-Z`)
- **THEN** subcommand routing still resolves for supported commands.

### Requirement: Cargo Runtime Safety Contracts
The cargo phase SHALL preserve global proxy safety guarantees.

#### Scenario: stderr diagnostics visibility
- **WHEN** cargo emits diagnostics on stderr
- **THEN** diagnostic signal remains visible in output (possibly in compacted form).

#### Scenario: non-zero exit parity
- **WHEN** wrapped cargo command exits non-zero
- **THEN** the same non-zero status is propagated.

#### Scenario: raw mode bypass
- **WHEN** `ccp` runs with `--raw`
- **THEN** cargo compaction is bypassed completely.

#### Scenario: low-confidence fallback
- **WHEN** output cannot be parsed with sufficient confidence for compaction
- **THEN** compaction falls back to passthrough output.

#### Scenario: ansi-aware normalization
- **WHEN** cargo emits ANSI/colorized output
- **THEN** filtered mode strips ANSI escape sequences before compaction logic.

#### Scenario: command context isolation
- **WHEN** cargo commands run adjacent to other tools
- **THEN** cargo filter state remains isolated to the cargo command context.
