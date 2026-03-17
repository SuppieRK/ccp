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
- SHOULD run `go build -o .bin/ccp ./cmd/ccp && PATH="$PWD/.bin:$PATH" go run ./cmd/ccp-ci -tool <tool> -artifacts-dir .artifacts/benchmark-<tool>` before changing an existing tool filter to establish a local baseline when feasible.
- MUST run `go build -o .bin/ccp ./cmd/ccp && PATH="$PWD/.bin:$PATH" go run ./cmd/ccp-ci -tool <tool> -artifacts-dir .artifacts/benchmark-<tool>` after changing a tool filter or benchmarked execution behavior.
- SHOULD investigate any non-zero benchmark exit and make the cause explicit in the final report.
- MUST use the generated `ccp gain` output, not just the harness summary, to evaluate compression results for the changed tool.
- MUST investigate regressions when a before-change baseline exists, and MUST highlight runs with no `gain.db` or no meaningful compression result for the changed tool.

---

# OpenSpec Synchronization

- ANY code modification requires matching OpenSpec updates.
- MUST keep specs, fixtures, tests, and implementation aligned.

## Canonical Extension Path

- New built-in filters MUST be authored in `filters/<tool>.yaml` and routed through `filters/.mappings.yaml` when wrapper or alias spellings need the same canonical filter.
- New built-in lifecycle agent adapters MUST register through `internal/lifecycle/agents/agent_specs.go` and the matching family or bespoke adapter implementation.
- Filter work MUST prefer extending the existing YAML DSL or shared runtime/helper layer before adding new bespoke Go behavior.
- Direct-tool and wrapper-tool variants SHOULD use separate YAML files when they own materially different behavior; use `.mappings.yaml` reuse only for true aliases or wrapper spellings that intentionally share one filter.
- Agent adapter work MUST prefer the existing managed context/rule/plugin/hook recipe surfaces for simple integrations; keep bespoke adapter implementations only when behavior is materially different.
- Helper placement MUST follow scope: shared engine behavior in `internal/contracts`, `internal/engine`, or `internal/filters`; lifecycle-agent helper behavior in `internal/lifecycle/agents`; broadly reusable low-level helpers only when the scope is clearly cross-cutting.
- Tests SHOULD follow source ownership: shared helpers in common test helper files, source-oriented suites near the owning code, and explicit cross-cutting exceptions only for fixture/integration/soak coverage.

---

# Runtime Rules

- Preserve native execution semantics: exit code, critical diagnostics, exact `--raw` behavior unless explicit redaction is enabled, and 0-byte output semantics.
- Fall back to passthrough on ambiguity, low confidence, or unsafe interactive/TTY-sensitive shapes.
- Favor shape-preserving compaction over representational rewrites when filtering is sufficient.
- Avoid re-implementing native tools when filtering suffices.
- Produce stable deterministic output with command-context isolation.
- MUST execute command shape exactly as typed unless filter contract defines normalization.
- MUST preserve native output affordances when possible, especially line-oriented forms that coding agents can reuse in follow-up shell expressions.
- MUST treat structured/precision modes as byte-preserving passthrough when required.
- `ccp capture` MUST preserve native command execution semantics while recording sequenced replay fixtures.

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
- Load [RELEASE](./docs/agent-rules/RELEASE.md) when modifying release, installer, or distribution logic.
- Load `use-modern-go` when adding, editing, or reviewing Go code and Go tests.
- Load `bdd` when adding or changing Ginkgo/Gomega coverage.
- If a referenced skill is unavailable, continue with the repository conventions in this file and the linked docs instead of blocking on the missing skill.

Cold-path governance rules are intentionally separated to reduce working-memory load.

## Skills

### Available skills

- `bdd`: Use for writing tests in Ginkgo/Gomega BDD-style (including table-driven tests).
- `use-modern-go`: Apply modern Go syntax and standard-library guidance based on the repository's detected Go version. Use for Go implementation work, Go test changes, and Go code review passes focused on outdated idioms. (files: `./.codex/skills/use-modern-go/SKILL.md`, `./.opencode/skills/use-modern-go/SKILL.md`)
