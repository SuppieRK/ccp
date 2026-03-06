# Agent Rule – Release

CI is canonical for release build commands and release publication behavior.

When modifying release logic, preserve:

- Tag resolution rules from `release-distribution.yml`: tags are required, must not start with `v`, and must start with a digit.
- Artifact naming: `ccp_<tag>_<os>_<arch>.zip`
- Build matrix targets: `linux`, `darwin`, `windows` x `amd64`, `arm64`
- Build flags: `CGO_ENABLED=0`, `-trimpath`, and version injection via `-X go-command-compression-proxy/internal/version.Version=<tag>`
- Architecture mapping used by release and install flows: `x86_64` -> `amd64`, `aarch64` -> `arm64`
- Release publication shape: archive uploads plus `ccp_checksums.txt`
- Smoke-install coverage for Unix installer flow and Windows zip installation

Installer mechanics MUST remain aligned with the release contract and published artifact names.
