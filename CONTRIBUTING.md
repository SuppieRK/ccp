# Contributing

Thank you for contributing to this project.

This repository develops a command compression proxy designed to reduce command output consumed by coding agents while preserving the signal required for correct automated decision-making. Human terminal fidelity is secondary to deterministic, machine-consumable correctness.

This document describes the contributor workflow for the current runtime and filter architecture.

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

The short version:

- the Go runtime under `internal/` owns command parsing, source ordering, stream handling, metrics, audit, and lifecycle behavior
- filter behavior is authored in YAML
- release builds load filters from project-local `./.ccp/filters` first and home-scoped `~/.config/ccp/filters` second
- project-local YAML overrides home-scoped YAML for both filter ids and `.mappings.yaml` aliases
- shipped filters in `filters/` are embedded into the binary and materialized into `~/.config/ccp/filters` through `ccp repair` and startup maintenance

---

## Required Local Validation

All contributors must run the following from the repository root before submitting changes:

```bash
./scripts/validate.sh
```

The script runs `gofmt`, `go vet`, `go test`, `go mod tidy`, `go test -race`, internal coverage-gate verification, and the local quality tools `staticcheck`, `ineffassign`, and `gocyclo` when they are installed. If one of those CLI tools is missing, the script prints an install suggestion instead of failing on the missing binary.

CI is authoritative. Pull requests must pass all CI checks.

Local release package instructions for end-to-end testing live in [LOCAL.md](./LOCAL.md).

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

## Filter Development Guidelines

When adding or modifying command filters:

- Author built-in filter behavior in `filters/<tool>.yaml`.
- Register wrapper or alias spellings through `filters/.mappings.yaml`.
- Prefer project-local authoring and debugging through `./.ccp/filters` when iterating on behavior.
- Prefer one family YAML definition when multiple subcommands share one logical behavior surface.
- Extend shared runtime behavior under `internal/contracts`, `internal/engine`, `internal/filters`, or `internal/filters/yaml` before adding bespoke tool-specific Go behavior.
- Keep filters small, focused, and responsibility-scoped.
- Widen corpus before widening claims. Real warning-bearing success paths, rich failures, and passthrough boundaries are often more important than squeezing a tiny clean fixture harder.
- Be cautious with table rewrites and log compression. If a native table is already compact, or if output is user-defined application logging, forcing a rewrite often harms utility more than it helps token count.

Current YAML scope and precedence:

- release builds load `./.ccp/filters` before `~/.config/ccp/filters`
- project-local filter definitions override home-scoped definitions with the same canonical filter id
- project-local `.mappings.yaml` aliases override home-scoped aliases with the same key
- shipped filters in `filters/` are reference material while developing in-repo and are the source embedded into release builds
- `ccp repair` rewrites the managed home-scoped filter state under `~/.config/ccp/filters`
- project-local filters remain user-managed and are not recreated automatically

Recommended local iteration loop:

```bash
ccp filter new my-tool
ccp capture -- my-tool ...
ccp verify
```

Recommended authoring flow:

1. Start with project-local authoring in `./.ccp/filters`.
2. Inspect the current schema and any existing project-local, home-scoped, or shipped filter/mapping definitions before changing behavior.
3. Capture real command output with `ccp capture`.
4. Iterate on the project-local filter and verify with `ccp verify`.
5. Once the behavior is sound, promote the change into `filters/<tool>.yaml` and `filters/.mappings.yaml` if it belongs in the shipped set.
6. Add or update replay fixtures under `testdata/benchmarks/<tool>/...` when the tool is benchmarked.

The scaffold generated by `ccp filter new` includes a `yaml-language-server` schema comment pointing at `schemas/ccp-filter.schema.json`.

## Adapter Development Guidelines

- Register new built-in adapters through `internal/lifecycle/agents/agent_specs.go`.
- Prefer recipe/config-driven adapters for simple managed instruction files, plugin files, or managed settings integrations.
- Keep bespoke adapter structs only where custom install, verify, or uninstall behavior materially differs from the shared families.
- Reuse shared managed-file, hook/settings, and instruction-block helpers before adding new per-adapter mutation code.

---

# Testing Expectations

Testing is required for all behavioral modifications.

Contributors MUST:

- Add or update unit tests for filter logic.
- Add planning tests when execution planning changes.
- Add runner integration tests when runtime behavior changes.
- Maintain replay benchmark fixtures under `testdata/benchmarks/<tool>/<case>/`.
- Prefer source-oriented suites with shared table/helper coverage instead of wrapper-only duplicate tests.
- Replace trivial recipe-backed adapter tests with family conformance tests; keep bespoke adapter tests only for bespoke behavior.

Parent-command tests should contain minimal routing or passthrough validation only. Detailed behavioral coverage belongs in the command family that owns the behavior.

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

Command-specific behavior should remain in command-scoped suites, but shared planner/runtime scaffolding belongs in shared test helper files rather than repeated one-off helpers.

---

# Benchmarking

Benchmarking measures compaction effectiveness and execution overhead in agent workflows.

Contributors modifying compaction logic or execution behavior must ensure benchmarks remain valid.

Requirements:

- Benchmark assets live under `testdata/benchmarks/<tool>/`.
- Replay benchmark fixtures are stored per case under `testdata/benchmarks/<tool>/<case>/`.
- Each replay fixture uses:
  - required `command.yaml`
  - optional `stdout.txt`
  - optional `stderr.txt`
  - optional `output.txt`
- At least one of `stdout.txt`, `stderr.txt`, or `output.txt` must exist in each fixture directory.
- `stdout.txt` and `stderr.txt` are sequenced replay streams and must keep contiguous `00000|` ordering.
- Assertions must be deterministic and default to exact file equality when `output.txt` exists.
- Replay fixture outputs are derived from `ccp verify`; benchmarks do not execute copied project trees anymore.
- Token-oriented metrics are the primary KPI.
- Proxy overhead must be tracked.
- Realistic corpus selection matters. Prefer fixtures sourced from real command runs or checked-in research output over synthetic toy examples when evaluating safety boundaries.

---

## Running Benchmarks Locally

Benchmark verification is exercised through the repository's Ginkgo/Gomega test suites.

Run the focused benchmark-related suites directly when you are iterating on replay or harness behavior:

```bash
go test -count=1 ./internal/benchmark ./cmd/ccp-ci ./internal/lifecycle
```

For a full contributor check, use:

```bash
./scripts/validate.sh
```

Benchmark token metrics are derived from generated runtime artifacts rather than committed snapshots. The replay runner consumes fixture directories; it does not execute copied benchmark projects.

---

# Pull Request Checklist

Before submitting a pull request:

- Ensure behavior changes include tests and fixtures.
- Keep documentation and benchmark expectations updated.
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
