## Purpose
Define Maven filter metadata, prepare dispatch, and runtime compaction behavior.

## Requirements

### Requirement: Maven Tool Identity And Aliases
The Maven filter SHALL identify as `maven` and support Maven wrapper/native aliases.

#### Scenario: executable aliases
- **WHEN** executable is `mvnw`, `./mvnw`, `mvnw.cmd`, `./mvnw.cmd`, `mvn.cmd`, `./mvn.cmd`, `mvn.bat`, or `./mvn.bat`
- **THEN** the Maven filter contract is used.

### Requirement: Maven Prepare Dispatch Metadata
Prepare SHALL preserve args and annotate parallel intent in dispatch metadata.

#### Scenario: parallel dispatch marker
- **WHEN** args include `-T`, `-T...`, `--threads`, or `--threads=...`
- **THEN** dispatch key includes `maven|parallel=1`.

#### Scenario: non-parallel dispatch marker
- **WHEN** parallel flags are absent
- **THEN** dispatch key includes `maven|parallel=0`.

### Requirement: Shared Context Exit-Flush Model
Maven runtime handling SHALL use shared stdout/stderr context and flush only on exit.

#### Scenario: shared stream context
- **WHEN** stdout and stderr events belong to the same command
- **THEN** both streams resolve to one shared context key.

#### Scenario: pre-exit collection
- **WHEN** event type is line, tick, or EOF
- **THEN** event is collected.

#### Scenario: exit with empty buffer
- **WHEN** exit is received and buffered output is empty
- **THEN** no output is emitted.

#### Scenario: exit fallback on low confidence
- **WHEN** compaction reports low confidence
- **THEN** raw buffered output is flushed unchanged.

### Requirement: Maven Semantic Compaction
Compaction SHALL suppress low-signal transfer noise while retaining high-signal module and failure diagnostics.

#### Scenario: success scope collapse with identity
- **WHEN** module/goal boundaries are recognized and scope is successful
- **THEN** scope is collapsed as `[ok] <module> : <goal>`.

#### Scenario: transfer progress suppression on success
- **WHEN** transfer progress lines occur without transfer failure
- **THEN** transfer progress lines are omitted.

#### Scenario: transfer diagnostics retained on failure
- **WHEN** transfer failure markers are detected
- **THEN** nearby transfer context and failure diagnostics are retained.

#### Scenario: failure retention with scope marker
- **WHEN** failure markers (`[ERROR]`, `Failed to execute goal`, `Caused by:`, stack lines, `BUILD FAILURE`) are present
- **THEN** compact output retains failure diagnostics
- **AND** includes failed scope marker `[x] <module> : <goal>` when scope identity is available.

#### Scenario: reactor summary priority
- **WHEN** reactor summary/final status lines are emitted
- **THEN** reactor summary and final status lines are retained in output.

### Requirement: Parallel Safety And Thread-Prefix Dedupe
Parallel-mode compaction SHALL fall back on low confidence and normalize thread prefixes for dedupe when compaction proceeds.

#### Scenario: low-confidence parallel interleaving
- **WHEN** dispatch indicates parallel mode and multiple thread prefixes appear before recognizable scope boundaries
- **THEN** compaction returns low confidence and raw output is flushed unchanged.

#### Scenario: thread-prefix dedupe normalization
- **WHEN** semantically identical lines differ only by leading thread prefixes
- **THEN** those lines are deduped as one semantic line.
