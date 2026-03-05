## Purpose
Define `go build` subfilter prepare and runtime behavior.

## Requirements

### Requirement: Build Prepare Structured-Output Safety
`go build` SHALL passthrough structured output flags in prepare.

#### Scenario: Structured output flag
- **WHEN** args include `-json` or `--json...`
- **THEN** prepare forces passthrough
- **AND** marks result ambiguous with reason `structured output mode`.

### Requirement: Build Runtime Stderr Handling
`go build` SHALL treat stderr differently for normal mode and trace mode.

#### Scenario: Normal stderr line passthrough
- **WHEN** stream is stderr, dispatch is not trace mode, and event is line
- **THEN** line is emitted immediately unchanged.

#### Scenario: Trace-mode stderr EOF compaction
- **WHEN** stream is stderr, dispatch includes `x=1`, and EOF is received with non-empty buffered content
- **THEN** output is compacted deterministically
- **AND** raw buffered content is flushed unchanged if compaction cannot be produced.
- **AND** recognized compact categories include build trace lines, `go: downloading ...` dependency lines, and diagnostic lines (`.go:<line>[:<col>]`, failure markers).
- **AND** rendered output includes a deterministic summary header plus optional trace/download info lines before bounded diagnostics.
- **AND** non-matching lines are ignored while reject-classified lines force passthrough fallback.

#### Scenario: Trace-mode stderr empty EOF
- **WHEN** stream is stderr, dispatch includes `x=1`, and EOF is received with empty buffered content
- **THEN** no output is emitted.

### Requirement: Build Runtime Stdout Exit Compaction
`go build` SHALL compact collected stdout on exit and preserve empty-output parity.

#### Scenario: EOF on stdout defers output
- **WHEN** stream is stdout and EOF is received
- **THEN** event is collected (no flush at EOF).

#### Scenario: Exit with non-empty buffered stdout
- **WHEN** stream is stdout, exit is received, and buffered output is non-empty
- **THEN** output is compacted deterministically
- **AND** raw buffered content is flushed unchanged if compaction cannot be produced.

#### Scenario: Exit with empty buffered stdout
- **WHEN** stream is stdout, exit is received, and buffered output is empty
- **THEN** no output is emitted.
