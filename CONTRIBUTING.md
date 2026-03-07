# Contributing

Thank you for contributing to this project.

This repository develops an agent-first command proxy designed to reduce command output consumed by coding agents while preserving the signal required for correct automated decision-making. Human terminal fidelity is secondary to deterministic, machine-consumable correctness.

This document describes expectations for contributors working in an open source environment while maintaining alignment with agent-oriented development constraints defined in `AGENTS.md`.

---

# Project Principles

Contributors should optimize for useful output reduction while preserving behavioral equivalence with wrapped native commands.

Contributions MUST:

- Reduce output only when correctness is preserved.
- Prefer semantic, shape-preserving filtering over terminal-faithful formatting or denser representational rewrites.
- Preserve native execution invariants: exit codes, critical diagnostics, exact `--raw` behavior unless explicit redaction is enabled, zero-byte output semantics, and safe passthrough on ambiguity.
- Preserve deterministic behavior: stable output for identical inputs, no cross-command state leakage, and reproducible compaction behavior.
- Avoid re-implementing native tooling when output filtering is sufficient.

Contributions MUST NOT:

- Retain verbosity solely for human readability.
- Trade correctness for compaction gains.
- Introduce compressed output forms that make line-oriented shell reuse or coding-agent interpretation materially harder without explicit spec justification.

---

# Development Environment

## Language

- Go 1.26 or newer is required.
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
./scripts/validate.sh
```

The script runs `gofmt`, `go vet`, `go test`, `go mod tidy`, `go test -race`, internal coverage-gate verification, and the local quality tools `staticcheck`, `ineffassign`, and `gocyclo` when they are installed. If one of those CLI tools is missing, the script prints an install suggestion instead of failing on the missing binary.

CI is authoritative. Pull requests must pass all CI checks.

Coverage gate expectation:
- `internal/...` package coverage and aggregate module-group coverage must remain at or above `80%`.

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
