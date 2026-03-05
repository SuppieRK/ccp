## Purpose
Define npm filter metadata, prepare routing, and runtime compaction behavior.

## Requirements

### Requirement: npm Tool Identity And Aliases
The npm filter SHALL identify as `npm` and support platform aliases.

#### Scenario: alias executables
- **WHEN** executable is `npm.cmd`, `./npm.cmd`, `npm.exe`, or `./npm.exe`
- **THEN** the npm filter contract is used.

### Requirement: npm Prepare Run-Mode Routing
Prepare SHALL scope compaction to `npm run` flows and passthrough others.

#### Scenario: run-mode dispatch
- **WHEN** args begin with `run`
- **THEN** prepare preserves args and sets dispatch key `npm|mode=run`.

#### Scenario: non-run passthrough
- **WHEN** args are empty or first arg is not `run`
- **THEN** prepare forces passthrough.

### Requirement: Shared Context Exit-Flush Model
npm runtime handling SHALL use shared stdout/stderr context and flush on exit.

#### Scenario: shared stream context
- **WHEN** stdout and stderr events belong to the same npm command
- **THEN** both streams resolve to one shared context key.

#### Scenario: pre-exit collection
- **WHEN** event type is line, tick, or EOF
- **THEN** event is collected.

#### Scenario: empty output success marker
- **WHEN** exit is received with empty buffered output and exit code is zero
- **THEN** output is `ok`.

#### Scenario: empty output non-zero
- **WHEN** exit is received with empty buffered output and exit code is non-zero
- **THEN** no output is emitted.

### Requirement: npm Compaction Classes
Compaction SHALL suppress lifecycle/progress noise and retain actionable output.

#### Scenario: lifecycle boilerplate suppression
- **WHEN** output contains lifecycle launcher lines (`> package@version script`, leading `> ...`, yarn-run launcher lines, `$ ...`)
- **THEN** those lines are removed.

#### Scenario: progress/noise suppression
- **WHEN** output contains progress-only artifacts (including carriage-return spinner updates)
- **THEN** those lines are removed.

#### Scenario: actionable warning retention
- **WHEN** output contains warning lines (`npm WARN`, `warning ...`)
- **THEN** warnings remain visible with retain-first dedupe.

#### Scenario: failure diagnostics retention
- **WHEN** output contains failure diagnostics (`npm ERR!`, explicit failure/error markers)
- **THEN** failure diagnostics remain visible.

#### Scenario: repeated debug-log footer dedupe
- **WHEN** npm failure footer/debug-log pointer lines repeat
- **THEN** repeated footer lines are deduped while retaining one copy.

### Requirement: Exit-Aware Output Normalization
Exit status SHALL gate synthetic success marker behavior.

#### Scenario: success with fully suppressed content
- **WHEN** compaction yields no retained lines and exit code is zero
- **THEN** output is `ok`.

#### Scenario: non-zero with fully suppressed content
- **WHEN** compaction yields no retained lines and exit code is non-zero
- **THEN** raw buffered output is flushed unchanged.

#### Scenario: low-confidence fallback
- **WHEN** output is low-confidence (for example NUL/control-content threshold)
- **THEN** raw buffered output is flushed unchanged.
