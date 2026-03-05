## Purpose
Define `deno` filter command scope, safety boundaries, and deterministic runtime compaction behavior.

## Requirements

### Requirement: Direct Deno Command Scope
The `deno` phase SHALL apply only to direct Deno command execution forms.

#### Scenario: direct deno invocation
- **WHEN** command planning resolves executable as `deno` (or platform alias such as `deno.exe`)
- **THEN** the `deno` filter path is eligible for compaction.

#### Scenario: non-deno invocation
- **WHEN** command planning resolves a different executable
- **THEN** the `deno` phase SHALL NOT claim ownership.

#### Scenario: interactive or task-oriented deno shapes
- **WHEN** invocation subcommand is `repl` or `task`
- **THEN** the system SHALL use passthrough behavior for safety.

### Requirement: Subcommand-Aware Dispatch
The `deno` phase SHALL use subcommand-aware preparation to select conservative compaction behavior.

#### Scenario: deno test mode
- **WHEN** invocation subcommand is `test`
- **THEN** test-output compaction MAY be applied while preserving failures and diagnostics.

#### Scenario: deno lint mode
- **WHEN** invocation subcommand is `lint`
- **THEN** lint-output folding MAY be applied while preserving diagnostics.

#### Scenario: deno check mode
- **WHEN** invocation subcommand is `check`
- **THEN** check-output folding MAY be applied while preserving diagnostics.

#### Scenario: deno run mode
- **WHEN** invocation subcommand is `run`
- **THEN** compaction is limited to runtime/progress/cache boilerplate classes.

### Requirement: Conservative Progress-Noise Compaction
The `deno` phase SHALL compact only low-risk, repetitive progress/cache noise.

#### Scenario: repeated fetch/cache/progress lines
- **WHEN** output contains repeated dependency download/cache/progress lines
- **THEN** those lines MAY be deduplicated with retain-first behavior.

#### Scenario: recognized dependency-progress prefixes
- **WHEN** output lines match Deno progress prefixes such as `Download`, `Check`, `Compile`, or `Bundle`
- **THEN** those lines MAY be compacted as low-signal progress/cache boilerplate.
- **AND** lifecycle classification SHALL use a case-sensitive prefix contract equivalent to:
  - `^(Download|Check|Compile|Bundle)\s+(http|https|file)://`

#### Scenario: carriage-return spinner/progress artifacts
- **WHEN** output contains progress artifacts driven by carriage returns (`\r`) with no actionable diagnostics
- **THEN** those artifacts MAY be suppressed.

### Requirement: Failure-First Diagnostic Retention
The `deno` phase SHALL preserve actionable diagnostics over compaction.

#### Scenario: failure diagnostics present
- **WHEN** output contains failure signal (`error`, `failed`, panic/stack markers, file:line references)
- **THEN** those lines remain visible after compaction.

#### Scenario: stderr diagnostic visibility
- **WHEN** Deno emits diagnostics on `stderr`
- **THEN** those diagnostics remain visible and are not dropped by compaction.

#### Scenario: panic high-signal output
- **WHEN** output contains high-signal panic/backtrace markers (for example `panic:` or `stack backtrace:`)
- **THEN** those lines SHALL bypass folding and remain visible immediately.

### Requirement: Structured Output Safety
The `deno` phase SHALL preserve structured-output integrity.

#### Scenario: structured output requested
- **WHEN** invocation includes structured-output flags (for example `--json`)
- **THEN** the system SHALL use passthrough behavior for safety.
- **AND** specialized deno compaction SHALL be neutralized for safety-focused line-wise passthrough.

### Requirement: Passthrough Safety Boundaries
The `deno` phase SHALL prefer passthrough when confidence is low.

#### Scenario: low-confidence output classification
- **WHEN** output shape is ambiguous or binary/control-heavy
- **THEN** the system SHALL return the un-compacted collected output block (no specialized deno compaction).

#### Scenario: raw mode enabled
- **WHEN** `ccp` is invoked with `--raw`
- **THEN** deno compaction is bypassed entirely.

#### Scenario: interactive permission prompt
- **WHEN** output contains interactive permission prompt lines (for example ending with `? [y/n/... ]`)
- **THEN** pending low-signal buffers SHALL flush and the prompt SHALL be emitted immediately.

### Requirement: Exit-Code Parity
The `deno` phase SHALL preserve process status semantics.

#### Scenario: non-zero deno exit
- **WHEN** wrapped Deno execution exits non-zero
- **THEN** the same non-zero status is propagated.

### Requirement: Empty-Result Success Normalization
The `deno` phase SHALL emit canonical success marker when successful output is fully suppressed.

#### Scenario: success with fully suppressed low-signal output
- **WHEN** command exits successfully and all emitted lines were safely compacted away
- **THEN** output is normalized to `ok`.

#### Scenario: deno test failure must block success marker
- **WHEN** invocation subcommand is `test` and any suite/test failure signal is present
- **THEN** output SHALL NOT be normalized to `ok` even if low-signal lines are suppressed.

### Requirement: Deterministic Validation in CI
The system SHALL validate deno behavior using fixture-backed checks.

#### Scenario: deno phase acceptance in CI
- **WHEN** CI runs deno fixture validation
- **THEN** correctness checks pass and measured compression meets per-tool threshold expectations for compacted flows.
