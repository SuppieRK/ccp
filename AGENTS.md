# AGENTS.md – cmdshape

If instructions conflict:
1. Follow explicit user instructions.
2. Then follow this file.
3. Then follow `CONTRIBUTING.md`.

CI is the canonical definition of release build mechanics.

---

# Validation

- All commands are executed from the repository root.
- MUST use Go 1.26+.
- MUST run `./scripts/validate.sh` from repository root ONLY if Go source code was changed.
- MUST treat any non-zero exit from the validation script as a failed validation.
- If the validation script reports missing optional tools, SHOULD suggest installing them.

# Versioning

- Repository version strings MUST use plain semantic versions in `X.Y.Z` form with no leading `v` prefix.
- Any version string that is not exactly `X.Y.Z` MUST be treated as invalid rather than coerced.

# Runtime Rules

- Preserve native execution semantics: exit code, critical diagnostics, exact `--raw` behavior unless explicit redaction is enabled, and 0-byte output semantics.
- Fall back to passthrough on ambiguity, low confidence, or unsafe interactive/TTY-sensitive shapes.
- Favor shape-preserving compaction over representational rewrites when filtering is enough.
- Avoid re-implementing native tools when filtering suffices.
- Produce stable deterministic output with command-context isolation.
- MUST execute the command shape exactly as typed unless the filter contract defines normalization.
- MUST preserve native output affordances when possible, especially line-oriented forms that coding agents can reuse in follow-up shell expressions.
- MUST treat structured/precision modes as byte-preserving passthrough when required.
- `cmdshape capture` MUST preserve native command execution semantics while recording sequenced replay fixtures.

---

# Agent Prohibitions (Explicit Failure Guards)

Agents MUST NOT:

- Modify generated benchmark artifacts manually.
- Bypass benchmark gate logic.
- Remove fallback safety behavior.
- Relax exit-code parity.
- Introduce non-native output syntaxes that make downstream shell filtering or coding-agent interpretation harder unless explicitly justified by spec.

---

# Scoped Rules

- Load [FILTERS](./docs/agent-rules/FILTERS.md) when adding or changing YAML filters, mappings, filter fixtures, or local filter authoring workflow.
- Load [AGENT_INTEGRATIONS](./docs/agent-rules/AGENT_INTEGRATIONS.md) when adding or changing lifecycle agent adapters, managed integration artifacts, or adapter registration.
- Load [TESTING](./docs/agent-rules/TESTING.md) when adding or changing tests, or when a change affects planner, runner, or cross-tool test coverage.
- Load [BENCHMARKS](./docs/agent-rules/BENCHMARKS.md) on any filter change, and when changing benchmark fixtures, benchmark harness behavior, or tool benchmark expectations.
- Load [RELEASE](./docs/agent-rules/RELEASE.md) when modifying release, installer, or distribution logic.
- Load [SITE](./docs/agent-rules/SITE.md) when modifying the GitHub Pages site, landing-page copy, site assets, or `site/` CSS/JS/layout.
- Load `use-modern-go` when adding, editing, or reviewing Go code and Go tests.
- Load `bdd` when adding or changing Ginkgo/Gomega coverage.
- If a referenced skill is unavailable, continue with the repository conventions in this file and the linked docs instead of blocking on the missing skill.

Cold-path governance rules are intentionally separated to reduce a working-memory load.

## Skills

### Available skills

- `bdd`: Use for writing tests in Ginkgo/Gomega BDD-style (including table-driven tests).
- `use-modern-go`: Apply modern Go syntax and standard-library guidance based on the repository's detected Go version. Use for Go implementation work, Go test changes, and Go code review passes focused on outdated idioms. (files: `./.codex/skills/use-modern-go/SKILL.md`, `./.opencode/skills/use-modern-go/SKILL.md`)
