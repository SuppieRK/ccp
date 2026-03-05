## Purpose
Define `kubectl get services` filter behavior.

## Requirements

### Requirement: Services Table Folding
`kubectl get services` SHALL fold repeated service signatures while preserving header/row integrity.

#### Scenario: Signature grouping
- **WHEN** services share the same fold signature (for example type + ports)
- **THEN** compact output emits deterministic grouped summaries.
- **AND** ports are read from the canonical services table `PORT(S)` column.

#### Scenario: All namespaces summary
- **WHEN** output includes `NAMESPACE` header
- **THEN** compact output summarizes services per namespace.

#### Scenario: Unknown header fallback
- **WHEN** first line is not a recognized services table header
- **THEN** output is passthrough.
