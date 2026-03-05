## Purpose
Define provenance attestation requirements for release binary archives published by this repository.

## Requirements

### Requirement: Release Binary Attestations
The release workflow SHALL generate GitHub attestations for published binary archive artifacts.

#### Scenario: Attestation generated for each released binary archive
- **WHEN** a tagged release workflow publishes `ccp_<version>_<os>_<arch>.zip` artifacts
- **THEN** the workflow generates a GitHub attestation for each published binary archive
- **AND** attestation subjects are selected from `./release/*.zip`.

#### Scenario: Attestation job has required GitHub permissions
- **WHEN** release publish/attestation steps run
- **THEN** workflow permissions include `attestations: write` and `id-token: write`
- **AND** attestation generation runs in the publish job that uploads release assets.

### Requirement: Attestation Scope Is Binary Archives Only
The attestation process SHALL cover binary archive artifacts only and MUST NOT include checksum files or auxiliary release metadata files.

#### Scenario: Checksum and metadata files excluded from attestation scope
- **WHEN** release artifacts include checksums or additional metadata outputs
- **THEN** attestation steps select only binary `.zip` release archives as attestation subjects.

### Requirement: Attestation Predicate Metadata
The attestation step SHALL emit repository-scoped predicate metadata for binary archive provenance.

#### Scenario: Predicate metadata fields are populated
- **WHEN** binary archive attestations are generated
- **THEN** predicate type is `https://schemas.go-command-compression-proxy.dev/release/binary-archive-attestation/v1`
- **AND** predicate includes repository/workflow/scope metadata
- **AND** scope value indicates binary-archive-only attestation intent.

### Requirement: Attestation Action Version Governance
The workflow SHALL pin `actions/attest` to a stable major version and SHALL enforce periodic review of that pinned version.

#### Scenario: Major-version pinning with review cadence
- **WHEN** provenance workflow configuration is updated or reviewed
- **THEN** `actions/attest` remains pinned to a major version reference
- **AND** maintainers review and update the pinned major version on a defined periodic cadence.

#### Scenario: Review reminder is recorded in workflow summary
- **WHEN** release publish workflow completes (including failure paths)
- **THEN** `$GITHUB_STEP_SUMMARY` includes an attestation-version review reminder
- **AND** reminder includes current pin, review cadence, and binary-archive scope note.
