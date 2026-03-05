# Contributing

Thank you for contributing to this project.

This repository develops an agent-first command proxy designed to reduce command output consumed by coding agents while preserving the signal required for correct automated decision-making.

Human terminal fidelity is secondary to deterministic, machine-consumable correctness.

This document describes expectations for contributors working in an open source environment while maintaining alignment with agent-oriented development constraints defined in `AGENTS.md`.

---

# Project Principles

## Optimization Target

The primary optimization goal is reduction of command output volume as a practical proxy for reduced token consumption in automated agent workflows.

Contributions MUST:

- Optimize for lower output bytes when correctness is preserved.
- Prefer semantic compaction over terminal-faithful formatting.
- Remove repetitive or low-signal output when safe.
- Preserve information required for correct downstream actions.

Contributions MUST NOT:

- Retain verbosity solely for human readability.
- Trade correctness for compaction gains.

---

## Correctness Expectations

All changes must preserve behavioral equivalence with wrapped native commands unless explicitly defined by filter behavior.

Contributors MUST ensure:

- Native command exit codes are preserved.
- Critical diagnostics remain visible, including error, failure, panic, and root-cause indicators.
- Raw execution mode remains byte-for-byte identical to native output.
- If native output is 0 bytes, proxy output must be 0 bytes.
- Ambiguous situations fall back safely to passthrough execution.
- Native tooling is not reimplemented when output filtering is sufficient.

---

## Determinism

Deterministic behavior is a core project requirement.

Changes MUST:

- Produce stable output for identical inputs.
- Avoid cross-command state leakage.
- Maintain reproducible compaction behavior.

---

# Development Environment

## Language

- Go 1.24 or newer is required.
- Code must be formatted using `gofmt`.
- Errors must be handled explicitly.
- Standard Go conventions for naming and package structure should be followed.

---

## Architecture

Complete architecture documentation is available in [ARCHITECTURE.md](./ARCHITECTURE.md).

---

## Required Local Validation

All contributors must run the following from the repository root before submitting changes:

```bash
gofmt -w $(find cmd internal -name '*.go')
go vet ./...
go test -count=1 ./...
go mod tidy
```

Recommended additional validation:

```bash
go test -count=1 -race ./...
go test -count=1 -v ./...
```

CI is authoritative. Pull requests must pass all CI checks.

Coverage gate expectation:
- `internal/...` package coverage and aggregate module-group coverage must remain at or above `80%`.

If coverage is below the limit, run:

```bash
mkdir -p .artifacts/coverage
# 1) Generate the coverprofile file used by the gate.
go test -count=1 -covermode=atomic -coverpkg=./internal/... -coverprofile=.artifacts/coverage/internal.cover ./...
# 2) Evaluate coverage thresholds from that generated coverprofile.
go run ./cmd/coverage-gate -coverprofile .artifacts/coverage/internal.cover -module go-command-compression-proxy -internal-prefix internal/ -threshold 80
```

Then add or update tests in the failing `internal/...` packages until the gate passes.

Required branch-protection checks should include PR validation jobs only (for example):
- `PR Validation / validate (ubuntu-latest)`
- `PR Validation / validate (macos-latest)`
- `PR Validation / validate (windows-latest)`

---

# Making Changes

## Scope Expectations

Pull requests should:

- Keep scope focused and clearly defined.
- Modify behavior intentionally and explicitly.
- Avoid unrelated refactoring.

---

## Specification Alignment

This repository uses [OpenSpec](https://openspec.dev/) as the behavioral contract system.

Contributors MUST:

- Maintain specs under `openspec/specs/`.
- Submit behavioral changes through `openspec/changes/`.
- Keep specifications, fixtures, tests, and implementation aligned.
- Ensure spec fixture directory names match OpenSpec spec identifiers.

---

## Filter Development Guidelines

When adding or modifying command filters:

- Single-command tools must contain one filter file and one matching test file.
- Parent-command tools should include one parent filter and parent test.
    - Parent tools must use `ToolFilterRegistry`.
    - Subcommand filters must live under `internal/engine/filters/<parent>/`.
- Each subcommand maps to a single filter and test file.
- Shared logic should live in `common.go` or `helpers.go`.
- Filters should remain small, focused, and responsibility-scoped.

---

# Testing Expectations

Testing is required for all behavioral modifications.

Contributors MUST:

- Add or update unit tests for filter logic.
- Add planning tests when execution planning changes.
- Add runner integration tests when runtime behavior changes.
- Maintain fixtures under `testdata/tool-fixtures/<spec-id>/<scenario>/`.

Parent-command specifications should contain minimal routing or passthrough validation only. Detailed behavioral coverage belongs in subcommand specifications.

Fixture naming must remain consistent with directory structure.

---

## Planning vs Runtime Changes

Planning changes affect execution planning decisions such as:

- tool selection
- argument handling
- dispatch behavior
- ambiguity handling
- strict-mode decisions
- passthrough selection

Runtime changes affect execution behavior such as:

- stdout and stderr handling
- compaction behavior
- raw-mode bypass
- exit-code propagation
- execution lifecycle behavior

Command-specific behavior must be tested in command-scoped test files rather than generic test suites.

---

# Benchmarking

Benchmarking measures compaction effectiveness and execution overhead in agent workflows.

Contributors modifying compaction logic or execution behavior must ensure benchmarks remain valid.

Requirements:

- Benchmark assets live under `testdata/tool-fixtures/<tool>/`.
- Scenario definitions are stored in `scenarios.json`.
- Benchmarks execute real commands against declared projects.
- Assertions must be deterministic.
- Token-oriented metrics are the primary KPI.
- Proxy overhead must be tracked.
- Runtime benchmark artifacts must not be committed to Git.

---

## Running Benchmarks Locally

Prerequisite tooling may include common development ecosystems such as Go, Docker, Node, Python, Rust, Deno, and related utilities depending on benchmark scenarios.

Run:

```bash
./scripts/benchmark-local.sh -t <tool>
```

Benchmark token metrics are derived from generated runtime artifacts rather than committed snapshots.

---

# Pull Request Checklist

Before submitting a pull request:

- Ensure behavior changes include tests and fixtures.
- Keep documentation and specifications updated.
- Pass required local validation checks.
- Describe behavioral impact and fallback boundaries clearly.

---

# Collaboration Expectations

Contributors are encouraged to:

- Prefer clarity over cleverness.
- Preserve deterministic system behavior.
- Discuss large or architectural changes before implementation.
- Raise ambiguities instead of making assumptions.

When repository conventions or requirements are unclear, open an issue or request clarification rather than guessing expected behavior.
