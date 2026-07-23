# CCP Filter Authoring Prompt

You are helping create or improve a CCP YAML filter for `{{FILTER_ID}}`.

When using the generic prompt, `<filter-id>` is a placeholder. Replace it with the lowercase filter id for the command you are working on.

## Hard Rules

- Start by working in the project-local filter directory: `./.ccp/filters`.
- If a matching global, home-scoped, or shipped filter already exists, copy it into `./.ccp/filters` first and edit that project-local copy.
- Do not edit global/home filters under `~/.config/ccp/filters` unless the user directly asks for a global filter change.
- Do not edit shipped built-in filters under `filters/` unless the user directly asks to change CCP's built-in filter set.
- Keep behavior in the YAML DSL whenever possible. Do not change Go runtime behavior unless the current DSL cannot express the requested behavior and the user confirms that broader change.
- Project-local filters remain inactive until the user approves their exact current bytes with `ccp filter trust`; never approve changed bytes without reviewing the complete source.

## Workflow

1. Inspect active filters with `ccp filter status`.
2. If `{{FILTER_ID}}` already exists outside `./.ccp/filters`, create `./.ccp/filters` and copy the existing YAML into `./.ccp/filters/{{FILTER_ID}}.yaml` before editing.
3. If no matching filter exists, run `ccp filter new {{FILTER_ID}}`.
4. Capture real command output with `ccp capture -- {{COMMAND_EXAMPLE}}`.
5. Edit only the project-local YAML filter and project-local `.mappings.yaml` as needed.
6. Run `ccp verify` or `ccp verify --dir <fixture-dir>`.
7. Compare `output.txt` with `verify-output.txt`, and inspect `verify-decisions.txt` when behavior is unclear.
8. After reviewing every project YAML file and `.mappings.yaml`, run `ccp filter trust` if the user wants this exact source activated.

## Performance-Guided Improvements

- Run `ccp filter performance --limit 30` to find local filter rows worth improving.
- For focused work on one command, run `ccp filter performance --tool <tool> --limit 30`. The `--tool` value is the invoked tool name, not necessarily the resolved filter id.
- Use `ccp filter performance --global --tool <tool> --limit 30` only when the user wants cross-workspace signal.
- Read `RUNS` as frequency, `SAVED` and `SAVINGS` as estimated compression value, `PASS` as passthrough rate, and `FAIL` as native command failure rate.
- Treat `review-case` hints as matched filters or cases with low savings; inspect whether the case should be improved, narrowed, or left alone.
- Treat `failure-heavy` hints as failure-output-heavy rows; do not optimize them unless the failure output is useful and representative.
- Treat `passthrough-opportunity` hints as frequent passthrough command shapes; capture real output before authoring a new case.
- Use performance as a prioritization signal, not proof. Copy or create the project-local filter first, then capture and verify representative output before changing behavior.

## Authoring Guidance

- Preserve native execution semantics: exit code, critical diagnostics, exact `--raw` behavior, and empty output behavior.
- Prefer passthrough for structured, precision-sensitive, interactive, ambiguous, or unsafe command shapes.
- Treat warning-bearing success output as important behavior, not noise to remove blindly.
- Avoid table rewrites unless the native table is truly noisy and the shorter form remains easy to use in follow-up commands.
- Preserve shell-usable output identity; if the result makes follow-up commands harder to form, prefer less compression.
- Use one hypothesis at a time and verify it against real captured output.
- When adding or changing shipped built-in filters, add or update benchmark fixtures under `testdata/benchmarks/<tool>/`.

## Useful Commands

```bash
ccp filter status
ccp filter performance --tool <tool> --limit 30
ccp filter new {{FILTER_ID}}
ccp filter trust
ccp capture -- {{COMMAND_EXAMPLE}}
ccp verify
ccp verify --dir <fixture-dir>
```
