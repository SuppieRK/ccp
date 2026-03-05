# Agent Rule – Release

CI is canonical for release build commands.

When modifying release logic:

MUST preserve:

- Artifact naming: `ccp_<tag>_<os>_<arch>.zip`
- Tag format without `v` prefix.
- Architecture mapping:
  `x86_64` -> `amd64`
  `aarch64` -> `arm64`
- Stable release resolution behavior.
- `CGO_ENABLED=0` requirement.

Installer mechanics MUST remain aligned with release contract.