# Agent Rule – Release

CI is canonical for release build commands and release publication behavior.

When modifying release logic, preserve:

- Tag resolution rules from `release-distribution.yml`: tags must already
  exist, use exact `X.Y.Z` syntax without `v`, and resolve to one immutable
  commit and tag object throughout every job.
- Artifact naming: `cmdshape_<tag>_<os>_<arch>.zip`
- Build matrix targets: `linux`, `darwin`, `windows` x `amd64`, `arm64`
- Build flags: `CGO_ENABLED=0`, `-trimpath`, and version injection via
  `-X github.com/SuppieRK/cmdshape/internal/version.Version=<tag>`
- Architecture mapping used by release and install flows: `x86_64` -> `amd64`, `aarch64` -> `arm64`
- Release publication shape: six archives plus `cmdshape_checksums.txt`
- Smoke-install coverage for cmdshape on Unix and Windows draft assets before
  publication. The `smoke-draft` job requires `contents: write` because GitHub
  exposes private draft releases only to tokens with push access; keep that
  permission scoped to the smoke job.
- Custom attestation predicate URI:
  `https://github.com/SuppieRK/cmdshape/attestations/binary-archive/v1`

Every release is cmdshape-only. Do not publish a legacy executable, checksum
manifest, alias, shim, or handoff release.

## Draft repair and public yank

The workflow deliberately refuses to overwrite either a draft or a public
release.

- If a pre-publication job fails after draft creation, inspect the private
  draft with `gh release view X.Y.Z --json isDraft,assets`. Delete only that
  draft with `gh release delete X.Y.Z --yes`, confirm the existing tag still
  resolves to the recorded source SHA, and rerun the workflow. Never move the
  tag to repair a draft.
- If a defect is discovered after publication, stop distribution, record the
  reason, and remove the release with `gh release delete X.Y.Z --yes` while
  preserving the immutable tag for provenance. Fix forward under a new patch
  version; do not reuse or retarget the published tag.
- Attestations bind archive digests and source SHA. A repaired draft must
  reproduce the same validated source identity; stale or mismatched assets
  must not be promoted.

Installer and self-upgrade mechanics MUST remain aligned with the cmdshape-only
release contract and published artifact names.
