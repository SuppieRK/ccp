## Purpose
Define required coverage thresholds and reporting modes for repository validation workflows.

## Requirements

### Requirement: Internal Coverage Threshold Enforcement
The system SHALL enforce minimum test coverage thresholds for `internal/...` in CI and local validation workflows.

#### Scenario: Per-package threshold failure
- **WHEN** any package under `internal/...` reports coverage below `80%`
- **THEN** the coverage gate fails
- **AND** the failing package and measured percentage are reported.

#### Scenario: Module-group threshold failure
- **WHEN** aggregate coverage for the `internal/...` module group is below `80%`
- **THEN** the coverage gate fails
- **AND** the module-group percentage is reported.

#### Scenario: Pull requests always enforce internal coverage gate
- **WHEN** pull request validation runs
- **THEN** internal coverage threshold checks are executed as required checks
- **AND** merge is blocked when those checks fail.

#### Scenario: Main validation enforces the same internal gate
- **WHEN** main validation runs for `push` events on `main`
- **THEN** internal coverage threshold checks run with the same `internal/...` gate threshold
- **AND** workflow validation fails when the gate fails.

### Requirement: Coverage Reporting Modes
The system SHALL support required and informational coverage reporting scopes.

#### Scenario: Required scope is internal
- **WHEN** coverage gates run in CI
- **THEN** only `internal/...` thresholds are treated as required pass/fail checks.

#### Scenario: Informational scope is outside required set
- **WHEN** packages outside `internal/...` are included in coverage output
- **THEN** their values are reported without failing the required gate.

#### Scenario: SonarQube analysis is informational in main validation
- **WHEN** main validation runs SonarQube analysis
- **THEN** analysis runs in scan/reporting mode using existing CI outputs
- **AND** Sonar analysis execution is non-blocking (`continue-on-error: true`)
- **AND** Sonar Quality Gate status is not required as a merge-blocking PR check.

#### Scenario: Coverage gate summary includes required and informational sections
- **WHEN** `cmd/coverage-gate` renders markdown summary output
- **THEN** it includes module-group coverage and per-package PASS/FAIL status for `internal/...`
- **AND** it includes an informational list for packages outside required scope.

### Requirement: SonarQube Project Wiring
CI SonarQube analysis SHALL use repository-scoped Sonar configuration and authentication settings.

#### Scenario: Sonar scan uses configured project and organization
- **WHEN** SonarQube analysis executes in GitHub Actions
- **THEN** it uses `sonar.projectKey=SuppieRK_ccp` and `sonar.organization=suppierk`
- **AND** authentication is provided through `SONAR_TOKEN` configured in GitHub Actions environment or secrets.

### Requirement: Coverage Remediation Guidance
The system SHALL document concrete remediation steps so contributors can restore coverage above the required threshold.

#### Scenario: Contributor guidance for coverage failures
- **WHEN** a contributor encounters internal coverage below the required threshold
- **THEN** repository contribution guidance includes commands to generate the internal coverage profile and run `cmd/coverage-gate`
- **AND** guidance states that tests must be added/updated until `internal/...` package and module-group coverage are each at least `80%`.
