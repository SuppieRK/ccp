## Purpose
Define `kubectl logs` filter behavior.

## Requirements

### Requirement: Logs Stream Handling
`kubectl logs` SHALL compact only by buffered flush behavior (no content rewrite), with follow-mode passthrough at prepare time.

#### Scenario: prepare follow passthrough
- **WHEN** args include `-f` or `--follow`
- **THEN** `Prepare(...)` returns `ForcePassthrough=true`.

#### Scenario: prepare non-follow compactable
- **WHEN** follow flags are absent
- **THEN** `Prepare(...)` preserves args and allows routed processing.

#### Scenario: stderr visibility
- **WHEN** stderr line events are received
- **THEN** lines are emitted immediately unchanged.

#### Scenario: stdout flush on EOF
- **WHEN** stdout reaches EOF with non-empty buffered output
- **THEN** buffered output is flushed unchanged.

#### Scenario: stdout empty EOF
- **WHEN** stdout reaches EOF with empty buffered output
- **THEN** no output is emitted.
