# Security Policy

## Project Status

This project is currently in an incubating state.

It follows zerover-style semantic versioning (0.y.z). All versions prior to 1.0.0 MUST be considered unstable. Backward compatibility is not guaranteed, and internal or external interfaces may change without notice.

Consumers MUST assume that any 0.x release may introduce breaking changes.

---

## Supported Versions

Only the latest released binary is supported.

| Version | Supported |
|---------|-----------|
| latest  | :white_check_mark: |
| older   | :x: |

There are no security patches for previous versions.

When a new release is published:

- The previous release is immediately unsupported.
- No backported fixes are issued.
- No LTS branches exist.
- No CVE backport commitments are made.

Users MUST upgrade to the latest release to receive security fixes.

---

## Security Scope

This project is a command proxy that:

- Executes native system commands.
- Processes stdout and stderr streams.
- May interact with local filesystems, Git repositories, container tooling, and language toolchains.

Security considerations therefore include:

- Command execution integrity.
- Argument normalization and dispatch safety.
- Stream handling correctness.
- Passthrough safety in ambiguous cases.
- Avoidance of command injection via filter logic.
- Preservation of native exit codes and diagnostics.

The project intentionally falls back to passthrough execution for ambiguous or unsafe command shapes to reduce risk amplification.

---

## Reporting a Vulnerability

If you discover a security vulnerability:

1. Do NOT open a public issue.
2. Open a private GitHub security advisory.
3. Provide:
   - A clear description of the issue.
   - Reproduction steps.
   - Impact assessment.
   - Affected version (if known).

Response expectations:

- Initial acknowledgment: within 72 hours.
- Triage and severity classification: best effort.
- Fix timeline: determined by impact and complexity.
- Disclosure: coordinated after fix release.

Because only the latest version is maintained, fixes will be released exclusively in the current development line.

---

## Disclosure Policy

After a fix is published:

- A public advisory may be issued.
- No backports will be created.
- Users are expected to upgrade immediately.

If a vulnerability affects runtime behavior guarantees (exit code parity, diagnostic preservation, raw-mode integrity, or deterministic execution), it will be treated as high severity.

---

## Security Boundaries

The project:

- Does NOT sandbox native commands.
- Does NOT provide isolation guarantees.
- Relies on the host operating system for process security.
- Executes commands with the invoking user’s privileges.

Users are responsible for:

- Running the proxy in trusted environments.
- Reviewing command inputs provided by automated agents.
- Applying least-privilege execution practices.

---

## Stability and Risk Notice

Due to incubating status and zerover versioning:

- APIs, CLI flags, and behavioral contracts may change.
- Security posture may evolve.
- Hardening is incremental.

Use in production environments requires independent risk evaluation.

Upgrading to the latest version is mandatory for continued security coverage.