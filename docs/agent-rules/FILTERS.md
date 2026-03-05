# Agent Rule – Filters

## Layout Invariants

Single-command tool:
- One filter file.
- One matching test file.

Parent-command tool:
- One parent filter file.
- One parent test file.
- Subcommands under: `internal/engine/filters/<parent>/`
- One subcommand per file.
- One subcommand test per file.
- MUST use `ToolFilterRegistry` for routing.

Shared helpers:
- `common.go` or `helpers.go` only.

## Spec Alignment

- Parent-command tools require parent and subcommand specs.
- Specs under: `openspec/specs/`
- Spec-fixture directory names MUST match spec IDs.
- When specs change, update: `internal/engine/filters/tool_fixtures_integration_test.go` (`toolSpecNames`).