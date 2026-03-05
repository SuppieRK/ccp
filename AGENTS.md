# AGENTS.md – Command Compression Proxy

This file defines hot-path operational invariants for automated coding agents.

If instructions conflict:
1. Follow explicit user instructions.
2. Then follow this file.
3. Then follow `CONTRIBUTING.md`.

CI configuration is the canonical definition of release build mechanics.

---

# Execution Environment Invariants

- MUST use Go 1.24+.
- MUST format with: `gofmt -w $(find cmd internal -name '*.go')`
- MUST run:
```go
go vet ./...
go test -count=1 ./...
go mod tidy
```
- All commands executed from repository root.

Optional: `go test -count=1 -race ./...`

---

# CI and Quality Gates

- MUST pass all tests.
- MUST pass cmd/coverage-gate for all pull requests.
- MUST keep `internal/...` package coverage and aggregate module-group coverage at or above `80%`.
- MUST have reproducible build artifacts.
- MUST use `CGO_ENABLED=0` for release binaries.
- MUST use deterministic benchmark assertions.
- MUST NOT commit runtime benchmark artifacts.

If coverage is below the threshold, MUST run and satisfy:
```bash
mkdir -p .artifacts/coverage
# 1) Generate the coverprofile file consumed by coverage-gate.
go test -count=1 -covermode=atomic -coverpkg=./internal/... -coverprofile=.artifacts/coverage/internal.cover ./...
# 2) Enforce thresholds against that generated coverprofile.
go run ./cmd/coverage-gate -coverprofile .artifacts/coverage/internal.cover -module go-command-compression-proxy -internal-prefix internal/ -threshold 80
```

---

# Mandatory OpenSpec Synchronization

- ANY code modification requires corresponding OpenSpec spec modification.
- No exceptions.
- PRs without matching spec updates are invalid.
- MUST keep specs, fixtures, tests, and implementation aligned.

---

# Runtime Behavioral Invariants (Non-Negotiable)

The proxy MUST:

- Preserve native command exit code.
- Preserve critical diagnostics.
- Keep `--raw` byte-for-byte equivalent.
- Fall back to passthrough on ambiguity or low confidence.
- Avoid re-implementing native tools when filtering suffices.
- Produce stable deterministic output for identical input.
- Maintain command-context isolation.
- Emit 0 bytes when native output is 0 bytes.

---

# Command Execution Constraints

- MUST execute command shape exactly as typed unless filter contract defines normalization.
- MUST treat structured/precision modes as byte-preserving passthrough when required.
- MUST keep interactive/TTY-sensitive commands in passthrough when unsafe.
- `--strict` MUST reject ambiguous plans.
- `--capture-raw` MUST preserve execution semantics.

---

# Release Logic Modification Constraints

If modifying release or installer logic, MUST preserve:

- Artifact format: `ccp_<tag>_<os>_<arch>.zip`
- Tag format without `v` prefix.
- Architecture mapping:
  `x86_64` -> `amd64`
  `aarch64` -> `arm64`
- Stable-release resolution behavior.
- `CGO_ENABLED=0` build requirement.

---

# Agent Prohibitions (Explicit Failure Guards)

Agents MUST NOT:

- Introduce new tooling without OpenSpec update.
- Restructure directories without corresponding spec modification.
- Modify generated benchmark artifacts manually.
- Bypass benchmark gate logic.
- Weaken deterministic guarantees.
- Remove fallback safety behavior.
- Relax exit-code parity.
- Silence diagnostics for compaction gain.

---

# Scoped Rule Retrieval

When modifying specific subsystems, retrieve and follow:

- [docs/agent-rules/TESTING.md](./docs/agent-rules/TESTING.md)
- [docs/agent-rules/BENCHMARKS.md](./docs/agent-rules/BENCHMARKS.md)
- [docs/agent-rules/RELEASE.md](./docs/agent-rules/RELEASE.md)
- [docs/agent-rules/FILTERS.md](./docs/agent-rules/FILTERS.md)

Cold-path governance rules are intentionally separated to reduce working-memory load.
