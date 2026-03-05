## Purpose
Define `go test` subfilter prepare and runtime compaction behavior.

## Requirements

### Requirement: Test Prepare Structured-Output Safety
`go test` SHALL passthrough structured output mode in prepare.

#### Scenario: Structured output flag
- **WHEN** args include `-json`
- **THEN** prepare forces passthrough
- **AND** marks result ambiguous with reason `structured output mode`.

### Requirement: Test Runtime Stream Handling
`go test` SHALL preserve stderr immediacy and compact collected stdout on EOF/exit.

#### Scenario: Stderr line passthrough
- **WHEN** stream is stderr and event is line
- **THEN** line is emitted immediately unchanged.

#### Scenario: Stdout collection on line events
- **WHEN** stream is stdout and event is line
- **THEN** output is collected.

#### Scenario: Empty buffered stdout on EOF/exit
- **WHEN** stream is stdout and EOF or exit is received with empty buffered output
- **THEN** no output is emitted.

### Requirement: Test Runtime Compaction
`go test` SHALL emit deterministic compact summaries when parsing is recognized.

#### Scenario: Pass summary
- **WHEN** recognized output contains only pass/no-test-files package summaries
- **THEN** output is `go test: <passed> passed, <no-test-files> no-test-files`.

#### Scenario: Failure summary
- **WHEN** recognized output contains failures
- **THEN** output includes retained failure lines
- **AND** summary is `go test: <passed> passed, <failed> failed, <no-test-files> no-test-files`.
- **AND** retained failure lines may include the immediate following source/indented context line when present.

#### Scenario: Parse-confidence fallback
- **WHEN** output is unrecognized or low-confidence (including NUL bytes)
- **THEN** raw buffered output is flushed unchanged.
