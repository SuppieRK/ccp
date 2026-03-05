## Purpose
Define `kubectl get nodes` filter behavior.

## Requirements

### Requirement: Nodes Table Folding
`kubectl get nodes` SHALL preserve non-ready rows and fold repetitive ready rows.

#### Scenario: All-ready summary
- **WHEN** all node rows are healthy/ready and share repeated signatures
- **THEN** output summarizes repeated healthy rows in compact grouped form.

#### Scenario: NotReady retention
- **WHEN** one or more rows are `NotReady`/unhealthy
- **THEN** those rows remain visible.
- **AND** healthy ready rows may be summarized.

#### Scenario: Unknown header fallback
- **WHEN** first line is not a recognized nodes table header
- **THEN** output is passthrough.
