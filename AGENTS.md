# AGENTS.md – Command Compression Proxy

If instructions conflict:
1. Follow explicit user instructions.
2. Then follow this file.
3. Then follow `CONTRIBUTING.md`.

CI is the canonical definition of release build mechanics.

---

# Validation

- All commands executed from repository root.
- MUST use Go 1.26+.
- MUST run `./scripts/validate.sh` from repository root.
- MUST treat any non-zero exit from the validation script as a failed validation.
- If the validation script reports missing optional tools, SHOULD suggest installing them.

---

# Benchmarks

- MUST NOT commit runtime benchmark artifacts.
- SHOULD run `./scripts/benchmark-local.sh -t <tool>` before changing an existing tool filter to establish a local baseline when feasible.
- MUST run `./scripts/benchmark-local.sh -t <tool>` after changing a tool filter or benchmarked execution behavior.
- MUST treat any non-zero benchmark exit as a failure.
- MUST use the script's `ccp gain` output, not just the harness summary, to evaluate compression results for the changed tool.
- MUST investigate regressions when a before-change baseline exists, and MUST highlight runs with no `gain.db` or no meaningful compression result for the changed tool.

---

# OpenSpec Synchronization

- ANY code modification requires matching OpenSpec updates.
- MUST keep specs, fixtures, tests, and implementation aligned.

---

# Runtime Rules

- Preserve native execution semantics: exit code, critical diagnostics, exact `--raw` behavior, and 0-byte output semantics.
- Fall back to passthrough on ambiguity, low confidence, or unsafe interactive/TTY-sensitive shapes.
- Favor shape-preserving compaction over representational rewrites when filtering is sufficient.
- Avoid re-implementing native tools when filtering suffices.
- Produce stable deterministic output with command-context isolation.
- MUST execute command shape exactly as typed unless filter contract defines normalization.
- MUST preserve native output affordances when possible, especially line-oriented forms that coding agents can reuse in follow-up shell expressions.
- MUST treat structured/precision modes as byte-preserving passthrough when required.
- `--strict` MUST reject ambiguous plans.
- `--capture-raw` MUST preserve execution semantics.

---

# Agent Prohibitions (Explicit Failure Guards)

Agents MUST NOT:

- Introduce new code without OpenSpec update.
- Modify generated benchmark artifacts manually.
- Bypass benchmark gate logic.
- Remove fallback safety behavior.
- Relax exit-code parity.
- Introduce non-native output syntaxes that make downstream shell filtering or coding-agent interpretation harder unless explicitly justified by spec.

---

# Scoped Rules

- Load [TESTING](./docs/agent-rules/TESTING.md) when adding or changing tests, or when a change affects planner, runner, or cross-tool test coverage.
- Load [BENCHMARKS](./docs/agent-rules/BENCHMARKS.md) when changing benchmark fixtures, benchmark harness behavior, or tool benchmark expectations.
- Load [FILTERS](./docs/agent-rules/FILTERS.md) when adding or changing command filters, filter fixtures, or filter-specific runner/benchmark coverage.
- Load [RELEASE](./docs/agent-rules/RELEASE.md) when modifying release, installer, or distribution logic.

Cold-path governance rules are intentionally separated to reduce working-memory load.
