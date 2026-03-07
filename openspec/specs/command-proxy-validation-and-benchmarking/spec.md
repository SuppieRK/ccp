## Purpose
Define validation and benchmarking workflow requirements that ensure proxy safety, deterministic fixture-based evaluation, and actionable CI reporting.

## Requirements

### Requirement: Scenario-Driven Benchmark Harness
The system SHALL provide a scenario-driven harness that validates proxy behavior against native command behavior for curated tool scenarios.

#### Scenario: Scenario contract is explicit and enforceable
- **WHEN** a scenario is defined in `testdata/tool-fixtures/<tool>/scenarios.json`
- **THEN** it includes `name`, `tool`, and `native` command entries
- **AND** optional fields (`project`, `expect_exit`, `must_contain`, `must_not_contain`, `ignore_lines`, `required`, `text_only`, `before_start`, `after_stop`, `structured_output`) are validated for shape and non-empty command tokens.

#### Scenario: Scenario ignore rules are deterministic regex filters
- **WHEN** scenario parity checks need to tolerate volatile lines
- **THEN** the scenario MAY define `ignore_lines` regex patterns
- **AND** each pattern MUST compile successfully
- **AND** only lines matching configured `ignore_lines` entries are excluded during native/proxy normalization.

#### Scenario: Scenario discovery is fixtures-root driven
- **WHEN** harness discovery runs for a fixtures root (including glob-expanded roots)
- **THEN** directories containing `scenarios.json` are treated as tool fixture roots
- **AND** each discovered scenario name is unique within its `scenarios.json` file.

#### Scenario: Project directory requirements depend on text-only mode
- **WHEN** a discovered scenario is not `text_only`
- **THEN** its project directory (tool dir or `project` relative path) MUST exist
- **AND WHEN** a discovered scenario is `text_only`
- **THEN** a missing project directory is allowed.

### Requirement: Fixture and Artifact I/O Contracts
The harness SHALL use stable fixture and artifact file contracts for replayability and CI outputs.

#### Scenario: Text-only fixture input/output contract
- **WHEN** a scenario is `text_only`
- **THEN** proxy output is read from `output.txt`
- **AND** native input is read from `input.txt` when present
- **AND** otherwise native input is reconstructed from sequenced `input-stdout.txt` and `input-stderr.txt`.

#### Scenario: Runtime artifact contract for benchmark runs
- **WHEN** benchmark scenarios execute in non-text-only mode
- **THEN** proxy output artifacts are written as `output.txt` under a per-scenario artifact directory
- **AND** capture-raw artifacts are normalized to `input-stdout.txt` and `input-stderr.txt` when capture-raw is supported.

#### Scenario: Capture-raw support is proxy-binary aware
- **WHEN** proxy binary is `ccp` or `ccp.exe`
- **THEN** harness executes a `--capture-raw` pass and collects input stream artifacts
- **AND WHEN** proxy binary is not recognized as `ccp`
- **THEN** capture-raw pass is skipped.

#### Scenario: Oversized artifacts are truncated deterministically
- **WHEN** generated artifact content exceeds `max-artifact-bytes`
- **THEN** content is truncated with a deterministic `[TRUNCATED]` suffix
- **AND** scenario warnings include truncation context.

### Requirement: Safety and Severity Evaluation
Benchmark results SHALL evaluate safety invariants first, then classify severity.

#### Scenario: Safety invariants govern scenario success
- **WHEN** a scenario is evaluated
- **THEN** safety requires exit-code parity with expected exit behavior
- **AND** `must_contain` and `must_not_contain` expectations are enforced on normalized proxy output
- **AND** `structured_output` scenarios require normalized native/proxy equality.

#### Scenario: Report schema includes native/proxy execution blocks
- **WHEN** benchmark results are serialized to `report.json`
- **THEN** each scenario result includes `native` and `proxy` objects
- **AND** each object includes `spec`, `exit_code`, `duration_ms`, `token_count`, and artifact references.

#### Scenario: Severity status is explicit and ordered
- **WHEN** summary tables are rendered
- **THEN** `Status` is the first column
- **AND** statuses are emitted as `green`, `yellow`, or `red`
- **AND** discrepancy context is emitted through scenario warnings and summary notes.

#### Scenario: Yellow overhead thresholds are configurable with defaults
- **WHEN** yellow-overhead environment variables are unset
- **THEN** defaults are used:
  - `BENCH_YELLOW_OVERHEAD_ABS_MS=20`
  - `BENCH_YELLOW_OVERHEAD_REL_PCT=25`
  - `BENCH_YELLOW_MIN_NATIVE_MS=50`.

#### Scenario: Yellow overhead requires absolute and relative regression
- **WHEN** scenario status is not already red
- **AND** scenario is not text-only
- **AND** native duration is at least `BENCH_YELLOW_MIN_NATIVE_MS`
- **AND** proxy overhead milliseconds are at least `BENCH_YELLOW_OVERHEAD_ABS_MS`
- **AND** proxy overhead ratio is at least `1 + BENCH_YELLOW_OVERHEAD_REL_PCT/100`
- **THEN** severity is classified as yellow.

### Requirement: Required-Scenario Benchmark Gating
Benchmark execution SHALL fail on required-scenario failures and any safety-invariant failure.

#### Scenario: Required scenarios are scenario-declared
- **WHEN** scenarios are loaded from fixtures
- **THEN** required benchmark gates are determined by each scenario's `required: true` flag
- **AND** no hardcoded fixed tool list is required by the harness.

#### Scenario: Required scenario failures fail the benchmark run
- **WHEN** any required scenario is unsuccessful
- **THEN** benchmark run reports `failed_required=true`
- **AND** benchmark command exits non-zero.

#### Scenario: Safety failures always fail benchmark run
- **WHEN** any scenario fails safety invariants
- **THEN** benchmark run reports `failed_safety=true`
- **AND** benchmark command exits non-zero regardless of required flag.

### Requirement: CI Reporting, Retention, and Quality Gates
CI workflows SHALL publish benchmark and coverage results with explicit retention and quality-gate behavior aligned to lifecycle stage.

#### Scenario: Main validation runs benchmark as informational
- **WHEN** main validation runs on `push` to `main`
- **THEN** benchmark harness execution is included in that workflow
- **AND** benchmark execution is configured as non-blocking (`continue-on-error: true`).

#### Scenario: Pull request validation does not orchestrate benchmark dispatch
- **WHEN** pull request validation runs
- **THEN** it does not include benchmark-required path classification, manual benchmark approval, benchmark dispatch, or benchmark-gate refresh jobs
- **AND** required PR checks remain limited to fast deterministic validation gates.

#### Scenario: Pull request benchmark scope is selected from changed files
- **WHEN** pull request validation runs benchmark discovery
- **THEN** changed files are analyzed to select impacted benchmark tools
- **AND** only selected tools are executed in benchmark matrix jobs
- **AND** broad core-path changes MAY trigger running all benchmark tools.

#### Scenario: SonarQube runs in main validation as informational
- **WHEN** main validation runs
- **THEN** SonarQube analysis is executed in that workflow
- **AND** SonarQube execution is configured as non-blocking (`continue-on-error: true`).

#### Scenario: SonarQube runs in PR validation as informational
- **WHEN** pull request validation runs
- **THEN** SonarQube analysis is executed in that workflow
- **AND** SonarQube execution is configured as non-blocking (`continue-on-error: true`).

#### Scenario: SonarQube job checks out full git history
- **WHEN** SonarQube analysis runs in CI
- **THEN** checkout uses full history (`fetch-depth: 0`) so SCM metadata is available
- **AND** shallow-clone warnings do not degrade issue assignment capabilities.

#### Scenario: Summary publication uses GitHub step summary
- **WHEN** benchmark harness and quality-gate steps complete
- **THEN** human-readable benchmark and gate summaries are appended to `$GITHUB_STEP_SUMMARY`.

#### Scenario: Artifact retention policies are explicit
- **WHEN** benchmark artifacts are uploaded by main validation
- **THEN** retention is explicitly configured
- **AND** retention duration remains documented and deterministic.

#### Scenario: Main benchmark compares against historical per-tool baselines
- **WHEN** main validation benchmark runs execute for tool fixtures
- **THEN** each tool run provides a `-previous-report` baseline input from persisted benchmark history
- **AND** successful push-to-main runs refresh per-tool history used for future comparisons.

#### Scenario: Main benchmark cache cleanup is non-blocking
- **WHEN** main validation attempts to delete an existing benchmark-history cache key before saving a refreshed entry
- **THEN** cache deletion failures (for example missing key responses) MUST NOT fail the workflow job
- **AND** benchmark artifact publication and subsequent cache save steps still run.

#### Scenario: Main benchmark executes one parallel job per tool
- **WHEN** main validation benchmark runs for discovered tool fixtures
- **THEN** CI schedules one benchmark matrix job per tool
- **AND** each job publishes tool-scoped benchmark artifacts independently.

#### Scenario: Benchmark matrix entries carry runtime requirements
- **WHEN** PR or main benchmark discovery selects tool fixtures for execution
- **THEN** each emitted benchmark matrix entry includes the tool identifier plus runtime requirement metadata
- **AND** benchmark jobs use that metadata to provision only the needed runtimes for that tool.

#### Scenario: Coverage and race gates are enforced in validation workflows
- **WHEN** PR/main validation runs
- **THEN** CI runs the repository local-validation helper covering `go vet ./...`, `go test -count=1 -race ./...`, internal coverage gate enforcement at `80%`, `staticcheck ./...`, `ineffassign ./...`, and `gocyclo -over 15 .`
- **AND** benchmark execution is part of main validation and does not block merge/release flow.
