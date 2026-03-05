## Purpose
Define `kubectl get pods` filter behavior.

## Requirements

### Requirement: Pods Table Folding
`kubectl get pods` SHALL keep table headers, preserve unhealthy rows, and fold repetitive healthy rows.

#### Scenario: Healthy row folding with anomaly retention
- **WHEN** output includes repeated healthy pod rows and one unhealthy row
- **THEN** unhealthy rows are preserved verbatim.
- **AND** healthy rows are folded into deterministic summaries.

#### Scenario: Unknown header fallback
- **WHEN** first line is not a recognized pods table header
- **THEN** output is passthrough.

#### Scenario: All namespaces summary
- **WHEN** output includes `NAMESPACE` header and healthy rows per namespace
- **THEN** healthy rows are summarized per namespace.

#### Scenario: All namespaces mixed-health summary
- **WHEN** output includes `NAMESPACE` header and a namespace has both healthy and unhealthy pod rows
- **THEN** namespace summary reports healthy and unhealthy counts.
