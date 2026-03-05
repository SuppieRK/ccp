## Purpose
Define Gradle filter routing metadata and runtime compaction behavior.

## Requirements

### Requirement: Gradle Tool Identity And Aliases
The Gradle filter SHALL identify as `gradle` and support wrapper executable aliases.

#### Scenario: Wrapper aliases
- **WHEN** executable name is `gradlew`, `./gradlew`, `gradlew.bat`, or `./gradlew.bat`
- **THEN** the Gradle filter contract is used.

### Requirement: Gradle Prepare And Context Strategy
The Gradle filter SHALL preserve args in prepare and use shared stdout/stderr context.

#### Scenario: Prepare passthrough normalization
- **WHEN** prepare runs for Gradle
- **THEN** normalized args match input args.

#### Scenario: Shared stream context
- **WHEN** stdout and stderr events belong to the same command
- **THEN** both streams resolve to one shared context key.

### Requirement: Gradle Exit-Time Flush Model
The Gradle filter SHALL collect line/tick/EOF events and flush only at exit.

#### Scenario: Pre-exit collection
- **WHEN** event type is line, tick, or EOF
- **THEN** event is collected.

#### Scenario: Exit with empty buffer
- **WHEN** exit is received and buffered output is empty
- **THEN** no output is emitted.

#### Scenario: Exit with non-empty buffer
- **WHEN** exit is received and buffered output is non-empty
- **THEN** output is compacted deterministically
- **AND** raw buffered output is flushed unchanged when compaction confidence is low.

### Requirement: Gradle Semantic Compaction
Gradle compaction SHALL suppress low-signal noise and retain high-signal diagnostics.

#### Scenario: Progress noise suppression
- **WHEN** download/progress lines are emitted
- **THEN** they are removed from compacted output.

#### Scenario: Successful task detail reduction
- **WHEN** `> Task :name` boundary transitions occur without failure for a task
- **THEN** that task is collapsed to `[ok] :name`.

#### Scenario: Failure-context retention
- **WHEN** failure markers, `Caused by:`, or help pointer lines are present
- **THEN** those lines are retained in compacted output.

#### Scenario: Framework stack suppression
- **WHEN** stack frames are framework/runtime noise (`org.gradle.*`, `java.base`, shutdown-hook noise)
- **THEN** those frames/lines are suppressed while user stack frames remain.

### Requirement: Interleaving Safety Fallback
Gradle compaction SHALL fall back to passthrough when interleaving confidence is low.

#### Scenario: Low-confidence interleaving
- **WHEN** task-prefixed lines indicate output from a different task interleaved into the current task block
- **THEN** compaction reports low confidence
- **AND** raw buffered output is flushed unchanged.
