## Purpose
Define `npx eslint` subfilter prepare defaults and runtime summarization behavior.

## Requirements

### Requirement: eslint Prepare JSON Defaults
The `npx eslint` route SHALL request JSON output by default.

#### Scenario: prepare format injection
- **WHEN** args do not specify formatter (`-f/--format`)
- **THEN** `-f json` is appended.

#### Scenario: explicit format preserved
- **WHEN** formatter is already provided
- **THEN** args are preserved.

### Requirement: eslint Runtime Stream Handling
The `npx eslint` route SHALL preserve stderr immediacy and flush stdout only on exit.

#### Scenario: stderr immediate visibility
- **WHEN** stderr line events are received
- **THEN** lines are emitted immediately unchanged.

#### Scenario: stdout collection pre-exit
- **WHEN** stdout receives tick/line events
- **THEN** events are collected.

#### Scenario: stdout EOF behavior
- **WHEN** stdout EOF is received before exit
- **THEN** no output is emitted.

### Requirement: eslint JSON Summarization
The `npx eslint` route SHALL strip npx wrapper noise and summarize parsed ESLint JSON diagnostics.

#### Scenario: wrapper-noise suppression
- **WHEN** buffered stdout includes npx wrapper prompt/install noise
- **THEN** wrapper lines are stripped before summary parsing.

#### Scenario: summarized diagnostics
- **WHEN** valid ESLint JSON is parsed with issues
- **THEN** output includes totals plus bounded top-rules/top-files summaries.
- **AND** per-file messages are rendered in stable line/column/rule order.
- **AND** summaries are capped to top 5 rules, top 5 files, and top 3 messages per rendered file.

#### Scenario: no-issue output suppression
- **WHEN** parsed JSON reports zero errors and zero warnings
- **THEN** no stdout output is emitted.

#### Scenario: parse fallback
- **WHEN** JSON parse fails
- **THEN** stripped raw output is flushed unchanged.
